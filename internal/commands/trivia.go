package commands

import (
	"context"
	stderrors "errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/yourusername/lolo/internal/database"
	boterrors "github.com/yourusername/lolo/internal/errors"
	"github.com/yourusername/lolo/internal/trivia"
)

const triviaCommandTimeout = 90 * time.Second

// TriviaCommand implements !trivia [topic].
type TriviaCommand struct {
	manager *trivia.Manager
}

func NewTriviaCommand(manager *trivia.Manager) *TriviaCommand {
	return &TriviaCommand{manager: manager}
}

func (c *TriviaCommand) Name() string {
	return "trivia"
}

func (c *TriviaCommand) Execute(ctx *Context) (*Response, error) {
	return executeTriviaStart(ctx, c.manager)
}

func (c *TriviaCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelNormal
}

func (c *TriviaCommand) Help() string {
	return "!trivia [topic] - Start a trivia round in this channel (reuse last topic if omitted)"
}

func (c *TriviaCommand) CooldownDuration() time.Duration {
	return 0
}

// QuizCommand implements !quiz [topic] as alias for !trivia.
type QuizCommand struct {
	manager *trivia.Manager
}

func NewQuizCommand(manager *trivia.Manager) *QuizCommand {
	return &QuizCommand{manager: manager}
}

func (c *QuizCommand) Name() string {
	return "quiz"
}

func (c *QuizCommand) Execute(ctx *Context) (*Response, error) {
	return executeTriviaStart(ctx, c.manager)
}

func (c *QuizCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelNormal
}

func (c *QuizCommand) Help() string {
	return "!quiz [topic] - Alias for !trivia (reuse last topic if omitted)"
}

func (c *QuizCommand) CooldownDuration() time.Duration {
	return 0
}

// CodeCommand implements !code [language].
type CodeCommand struct {
	manager *trivia.Manager
}

func NewCodeCommand(manager *trivia.Manager) *CodeCommand {
	return &CodeCommand{manager: manager}
}

func (c *CodeCommand) Name() string {
	return "code"
}

func (c *CodeCommand) Execute(ctx *Context) (*Response, error) {
	if ctx.IsPM {
		return NewErrorResponse("Code quiz can only be started in a channel."), nil
	}

	language := ""
	switch len(ctx.Args) {
	case 0:
		rememberedLanguage, ok := c.manager.GetLastCodeLanguage(ctx.Channel)
		if !ok {
			return nil, boterrors.NewInvalidSyntaxError("code", "!code <"+supportedCodeLanguagePattern()+">")
		}
		language = rememberedLanguage
	case 1:
		language = ctx.Args[0]
	default:
		return nil, boterrors.NewInvalidSyntaxError("code", "!code <"+supportedCodeLanguagePattern()+">")
	}

	commandCtx, cancel := context.WithTimeout(context.Background(), triviaCommandTimeout)
	defer cancel()

	message, err := c.manager.StartCodeRound(commandCtx, ctx.Channel, language)
	if err != nil {
		switch {
		case stderrors.Is(err, trivia.ErrUnsupportedCodeLanguage):
			return NewResponse("Unsupported language. Supported: " + supportedCodeLanguageSummary() + " (aliases: js, ts, py, sh, c++, c#)."), nil
		case stderrors.Is(err, trivia.ErrRoundAlreadyActive):
			return NewResponse("A trivia/code round is already active in this channel."), nil
		case stderrors.Is(err, trivia.ErrTriviaDisabled):
			return NewResponse("Trivia/code rounds are disabled in this channel."), nil
		case stderrors.Is(err, trivia.ErrGeneratorDisabled):
			return NewResponse("Code generation is unavailable right now. Check OpenAI configuration."), nil
		case stderrors.Is(err, trivia.ErrGenerationFailed):
			return NewResponse("Failed to generate a unique code question right now. Please try again."), nil
		case stderrors.Is(err, context.DeadlineExceeded):
			return NewResponse("Code generation timed out. Please try again."), nil
		default:
			return nil, boterrors.NewUnexpectedError(err)
		}
	}

	return NewResponse(message), nil
}

func (c *CodeCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelNormal
}

func (c *CodeCommand) Help() string {
	return "!code [<" + supportedCodeLanguagePattern() + ">] - Start a one-line coding quiz round (reuse last language if omitted)"
}

func (c *CodeCommand) CooldownDuration() time.Duration {
	return 0
}

func executeTriviaStart(ctx *Context, manager *trivia.Manager) (*Response, error) {
	if ctx.IsPM {
		return NewErrorResponse("Trivia can only be started in a channel."), nil
	}

	topic := strings.TrimSpace(strings.Join(ctx.Args, " "))
	if topic == "" {
		rememberedTopic, ok := manager.GetLastTriviaTopic(ctx.Channel)
		if !ok {
			return nil, boterrors.NewInvalidSyntaxError("trivia", "!trivia <topic>")
		}
		topic = rememberedTopic
	}

	commandCtx, cancel := context.WithTimeout(context.Background(), triviaCommandTimeout)
	defer cancel()

	message, err := manager.StartRound(commandCtx, ctx.Channel, topic)
	if err != nil {
		switch {
		case stderrors.Is(err, trivia.ErrRoundAlreadyActive):
			return NewResponse("A trivia/code round is already active in this channel."), nil
		case stderrors.Is(err, trivia.ErrTriviaDisabled):
			return NewResponse("Trivia/code rounds are disabled in this channel."), nil
		case stderrors.Is(err, trivia.ErrGeneratorDisabled):
			return NewResponse("Trivia generation is unavailable right now. Check OpenAI configuration."), nil
		case stderrors.Is(err, trivia.ErrGenerationFailed):
			return NewResponse("Failed to generate a unique trivia question right now. Please try again."), nil
		case stderrors.Is(err, context.DeadlineExceeded):
			return NewResponse("Trivia generation timed out. Please try again."), nil
		default:
			return nil, boterrors.NewUnexpectedError(err)
		}
	}

	return NewResponse(message), nil
}

// HintCommand implements !hint.
type HintCommand struct {
	manager *trivia.Manager
}

func NewHintCommand(manager *trivia.Manager) *HintCommand {
	return &HintCommand{manager: manager}
}

func (c *HintCommand) Name() string {
	return "hint"
}

func (c *HintCommand) Execute(ctx *Context) (*Response, error) {
	if ctx.IsPM {
		return NewErrorResponse("Hints are only available in a channel with an active trivia/code round."), nil
	}

	message, err := c.manager.UseHint(ctx.Channel)
	if err != nil {
		switch {
		case stderrors.Is(err, trivia.ErrNoActiveRound):
			return NewResponse("No active trivia/code round in this channel."), nil
		case stderrors.Is(err, trivia.ErrHintsDisabled):
			return NewResponse("Hints are disabled for this channel."), nil
		case stderrors.Is(err, trivia.ErrHintAlreadyUsed):
			return NewResponse("A hint has already been used this round."), nil
		default:
			return nil, boterrors.NewUnexpectedError(err)
		}
	}

	return NewResponse(message), nil
}

func (c *HintCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelNormal
}

func (c *HintCommand) Help() string {
	return "!hint - Reveal a hint for the active trivia/code round (if enabled)"
}

func (c *HintCommand) CooldownDuration() time.Duration {
	return 0
}

// TriviaRulesCommand implements !triviarules.
type TriviaRulesCommand struct {
	manager *trivia.Manager
}

func NewTriviaRulesCommand(manager *trivia.Manager) *TriviaRulesCommand {
	return &TriviaRulesCommand{manager: manager}
}

func (c *TriviaRulesCommand) Name() string {
	return "triviarules"
}

func (c *TriviaRulesCommand) Execute(ctx *Context) (*Response, error) {
	channel := ctx.Channel
	settings, err := c.manager.GetSettings(channel)
	if err != nil {
		return nil, boterrors.NewDatabaseError("get trivia settings", err)
	}

	return NewResponse(formatTriviaRules(settings)), nil
}

func (c *TriviaRulesCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelNormal
}

func (c *TriviaRulesCommand) Help() string {
	return "!triviarules - Show how trivia works"
}

func (c *TriviaRulesCommand) CooldownDuration() time.Duration {
	return 0
}

// QuizRulesCommand implements !quizrules alias.
type QuizRulesCommand struct {
	manager *trivia.Manager
}

func NewQuizRulesCommand(manager *trivia.Manager) *QuizRulesCommand {
	return &QuizRulesCommand{manager: manager}
}

func (c *QuizRulesCommand) Name() string {
	return "quizrules"
}

func (c *QuizRulesCommand) Execute(ctx *Context) (*Response, error) {
	channel := ctx.Channel
	settings, err := c.manager.GetSettings(channel)
	if err != nil {
		return nil, boterrors.NewDatabaseError("get trivia settings", err)
	}

	return NewResponse(formatTriviaRules(settings)), nil
}

func (c *QuizRulesCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelNormal
}

func (c *QuizRulesCommand) Help() string {
	return "!quizrules - Alias for !triviarules"
}

func (c *QuizRulesCommand) CooldownDuration() time.Duration {
	return 0
}

func formatTriviaRules(settings trivia.ChannelSettings) string {
	hintStatus := "enabled"
	if !settings.HintsEnabled {
		hintStatus = "disabled"
	}

	return fmt.Sprintf(
		"Trivia rules: Start with !trivia <topic> or !quiz <topic> (or omit topic to reuse last). Code mode: !code <lang> (or omit language to reuse last). "+
			"Answer by typing normally in channel (code answers must be one line). "+
			"Time limits: trivia %ds, code %ds. Scoring: faster answers earn more points (base %d, minimum %d). "+
			"Difficulties: trivia %s, code %s. Hints are %s and using !hint applies a -%d point penalty. "+
			"If nobody matches exactly, timeout may trigger strict close-answer judging. "+
			"Use !top10 for leaderboard and !score [nick] for scores.",
		settings.AnswerTimeSeconds,
		settings.CodeAnswerTimeSeconds,
		settings.BasePoints,
		settings.MinimumPoints,
		settings.Difficulty,
		settings.CodeDifficulty,
		hintStatus,
		settings.HintPenalty,
	)
}

func supportedCodeLanguageSummary() string {
	return strings.Join(trivia.SupportedCodeLanguages(), ", ")
}

func supportedCodeLanguagePattern() string {
	return strings.Join(trivia.SupportedCodeLanguages(), "|")
}

// Top10Command implements !top10.
type Top10Command struct {
	manager *trivia.Manager
}

func NewTop10Command(manager *trivia.Manager) *Top10Command {
	return &Top10Command{manager: manager}
}

func (c *Top10Command) Name() string {
	return "top10"
}

func (c *Top10Command) Execute(ctx *Context) (*Response, error) {
	if ctx.IsPM {
		return NewErrorResponse("Leaderboard is channel-specific. Use !top10 in a channel."), nil
	}

	entries, err := c.manager.GetTopScores(ctx.Channel, 10)
	if err != nil {
		return nil, boterrors.NewDatabaseError("get trivia leaderboard", err)
	}

	if len(entries) == 0 {
		return NewResponse("No trivia scores yet for this channel."), nil
	}

	parts := make([]string, 0, len(entries))
	rank := 0
	lastScore := 0
	for i, entry := range entries {
		if i == 0 || entry.Score < lastScore {
			rank = i + 1
			lastScore = entry.Score
		}
		parts = append(parts, fmt.Sprintf("%d. %s (%d)", rank, entry.Nick, entry.Score))
	}

	return NewResponse(fmt.Sprintf("Top 10 trivia scores for %s: %s", ctx.Channel, strings.Join(parts, " | "))), nil
}

func (c *Top10Command) RequiredPermission() database.PermissionLevel {
	return database.LevelNormal
}

func (c *Top10Command) Help() string {
	return "!top10 - Show top trivia scores for this channel"
}

func (c *Top10Command) CooldownDuration() time.Duration {
	return 0
}

// ScoreCommand implements !score [nick] and admin score management subcommands.
type ScoreCommand struct {
	manager *trivia.Manager
	db      *database.DB
}

func NewScoreCommand(manager *trivia.Manager, db *database.DB) *ScoreCommand {
	return &ScoreCommand{
		manager: manager,
		db:      db,
	}
}

func (c *ScoreCommand) Name() string {
	return "score"
}

func (c *ScoreCommand) Execute(ctx *Context) (*Response, error) {
	if ctx.IsPM {
		return NewErrorResponse("Scores are channel-specific. Use !score in a channel."), nil
	}

	if len(ctx.Args) == 0 {
		return c.showScore(ctx, ctx.Nick)
	}

	subcommand := strings.ToLower(ctx.Args[0])
	switch subcommand {
	case "set", "add", "remove", "reset":
		return c.adminScoreAction(ctx, subcommand)
	default:
		if len(ctx.Args) > 1 {
			return nil, boterrors.NewInvalidSyntaxError("score", "!score [nick]")
		}
		return c.showScore(ctx, ctx.Args[0])
	}
}

func (c *ScoreCommand) showScore(ctx *Context, nick string) (*Response, error) {
	score, found, err := c.manager.GetScore(ctx.Channel, nick)
	if err != nil {
		return nil, boterrors.NewDatabaseError("get trivia score", err)
	}
	if !found {
		score = 0
	}

	if strings.EqualFold(nick, ctx.Nick) {
		return NewResponse(fmt.Sprintf("Your trivia score in %s: %d", ctx.Channel, score)), nil
	}
	return NewResponse(fmt.Sprintf("%s's trivia score in %s: %d", nick, ctx.Channel, score)), nil
}

func (c *ScoreCommand) adminScoreAction(ctx *Context, action string) (*Response, error) {
	if ctx.UserLevel < database.LevelAdmin {
		return nil, boterrors.NewPermissionError(database.LevelAdmin)
	}

	switch action {
	case "set":
		if len(ctx.Args) != 3 {
			return nil, boterrors.NewInvalidSyntaxError("score", "!score set <nick> <points>")
		}
		nick := ctx.Args[1]
		points, err := parseNonNegativeInt(ctx.Args[2])
		if err != nil {
			return NewErrorResponse(err.Error()), nil
		}
		if err := c.manager.SetScore(ctx.Channel, nick, points); err != nil {
			return nil, boterrors.NewDatabaseError("set trivia score", err)
		}
		c.logAudit(ctx, "trivia_score_set", nick, fmt.Sprintf("channel=%s points=%d", ctx.Channel, points))
		return NewResponse(fmt.Sprintf("Set trivia score for %s in %s to %d.", nick, ctx.Channel, points)), nil
	case "add":
		if len(ctx.Args) != 3 {
			return nil, boterrors.NewInvalidSyntaxError("score", "!score add <nick> <points>")
		}
		nick := ctx.Args[1]
		points, err := parseNonNegativeInt(ctx.Args[2])
		if err != nil {
			return NewErrorResponse(err.Error()), nil
		}
		newTotal, err := c.manager.AddScore(ctx.Channel, nick, points)
		if err != nil {
			return nil, boterrors.NewDatabaseError("add trivia score", err)
		}
		c.logAudit(ctx, "trivia_score_add", nick, fmt.Sprintf("channel=%s add=%d total=%d", ctx.Channel, points, newTotal))
		return NewResponse(fmt.Sprintf("Added %d trivia points to %s in %s (new total: %d).", points, nick, ctx.Channel, newTotal)), nil
	case "remove":
		if len(ctx.Args) != 3 {
			return nil, boterrors.NewInvalidSyntaxError("score", "!score remove <nick> <points>")
		}
		nick := ctx.Args[1]
		points, err := parseNonNegativeInt(ctx.Args[2])
		if err != nil {
			return NewErrorResponse(err.Error()), nil
		}
		newTotal, err := c.manager.AddScore(ctx.Channel, nick, -points)
		if err != nil {
			return nil, boterrors.NewDatabaseError("remove trivia score", err)
		}
		c.logAudit(ctx, "trivia_score_remove", nick, fmt.Sprintf("channel=%s remove=%d total=%d", ctx.Channel, points, newTotal))
		return NewResponse(fmt.Sprintf("Removed %d trivia points from %s in %s (new total: %d).", points, nick, ctx.Channel, newTotal)), nil
	case "reset":
		if len(ctx.Args) != 2 {
			return nil, boterrors.NewInvalidSyntaxError("score", "!score reset <nick>")
		}
		nick := ctx.Args[1]
		if err := c.manager.ResetScore(ctx.Channel, nick); err != nil {
			return nil, boterrors.NewDatabaseError("reset trivia score", err)
		}
		c.logAudit(ctx, "trivia_score_reset", nick, fmt.Sprintf("channel=%s", ctx.Channel))
		return NewResponse(fmt.Sprintf("Reset trivia score for %s in %s.", nick, ctx.Channel)), nil
	default:
		return nil, boterrors.NewInvalidSyntaxError("score", "!score [nick] | !score set/add/remove/reset ...")
	}
}

func (c *ScoreCommand) logAudit(ctx *Context, action, target, details string) {
	if c.db == nil {
		return
	}
	if err := c.db.LogAuditAction(ctx.Nick, ctx.Hostmask, action, target, details, "success"); err != nil {
		fmt.Printf("Warning: failed to log audit action: %v\n", err)
	}
}

func (c *ScoreCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelNormal
}

func (c *ScoreCommand) Help() string {
	return "!score [nick] - Show trivia score | Admin: !score set/add/remove/reset <nick> <points>"
}

func (c *ScoreCommand) CooldownDuration() time.Duration {
	return 0
}

// TriviaSettingsCommand implements channel trivia settings management.
type TriviaSettingsCommand struct {
	manager *trivia.Manager
	db      *database.DB
}

func NewTriviaSettingsCommand(manager *trivia.Manager, db *database.DB) *TriviaSettingsCommand {
	return &TriviaSettingsCommand{
		manager: manager,
		db:      db,
	}
}

func (c *TriviaSettingsCommand) Name() string {
	return "triviasettings"
}

func (c *TriviaSettingsCommand) Execute(ctx *Context) (*Response, error) {
	if ctx.IsPM {
		return NewErrorResponse("Trivia settings are channel-specific. Use this command in a channel."), nil
	}

	if len(ctx.Args) == 0 || strings.EqualFold(ctx.Args[0], "show") {
		settings, err := c.manager.GetSettings(ctx.Channel)
		if err != nil {
			return nil, boterrors.NewDatabaseError("get trivia settings", err)
		}
		return NewResponse(formatSettingsMessage(ctx.Channel, settings)), nil
	}

	subcommand := strings.ToLower(ctx.Args[0])
	switch subcommand {
	case "time":
		if len(ctx.Args) != 2 {
			return nil, boterrors.NewInvalidSyntaxError("triviasettings", "!triviasettings time <seconds>")
		}
		seconds, err := parseNonNegativeInt(ctx.Args[1])
		if err != nil {
			return NewErrorResponse(err.Error()), nil
		}
		settings, err := c.manager.UpdateAnswerTime(ctx.Channel, seconds)
		if err != nil {
			return NewErrorResponse(err.Error()), nil
		}
		c.logAudit(ctx, "trivia_settings_time", fmt.Sprintf("channel=%s seconds=%d", ctx.Channel, seconds))
		return NewResponse(formatSettingsMessage(ctx.Channel, settings)), nil
	case "codetime":
		if len(ctx.Args) != 2 {
			return nil, boterrors.NewInvalidSyntaxError("triviasettings", "!triviasettings codetime <seconds>")
		}
		seconds, err := parseNonNegativeInt(ctx.Args[1])
		if err != nil {
			return NewErrorResponse(err.Error()), nil
		}
		settings, err := c.manager.UpdateCodeAnswerTime(ctx.Channel, seconds)
		if err != nil {
			return NewErrorResponse(err.Error()), nil
		}
		c.logAudit(ctx, "trivia_settings_code_time", fmt.Sprintf("channel=%s seconds=%d", ctx.Channel, seconds))
		return NewResponse(formatSettingsMessage(ctx.Channel, settings)), nil
	case "hint":
		if len(ctx.Args) != 2 {
			return nil, boterrors.NewInvalidSyntaxError("triviasettings", "!triviasettings hint on|off")
		}
		enabled, err := parseOnOff(ctx.Args[1])
		if err != nil {
			return NewErrorResponse(err.Error()), nil
		}
		settings, err := c.manager.UpdateHintsEnabled(ctx.Channel, enabled)
		if err != nil {
			return NewErrorResponse(err.Error()), nil
		}
		c.logAudit(ctx, "trivia_settings_hint", fmt.Sprintf("channel=%s hints_enabled=%t", ctx.Channel, enabled))
		return NewResponse(formatSettingsMessage(ctx.Channel, settings)), nil
	case "difficulty":
		if len(ctx.Args) != 2 {
			return nil, boterrors.NewInvalidSyntaxError("triviasettings", "!triviasettings difficulty <easy|medium|hard>")
		}
		difficulty, err := parseDifficulty(ctx.Args[1])
		if err != nil {
			return NewErrorResponse(err.Error()), nil
		}
		settings, err := c.manager.UpdateDifficulty(ctx.Channel, difficulty)
		if err != nil {
			return NewErrorResponse(err.Error()), nil
		}
		c.logAudit(ctx, "trivia_settings_difficulty", fmt.Sprintf("channel=%s difficulty=%s", ctx.Channel, difficulty))
		return NewResponse(formatSettingsMessage(ctx.Channel, settings)), nil
	case "codedifficulty":
		if len(ctx.Args) != 2 {
			return nil, boterrors.NewInvalidSyntaxError("triviasettings", "!triviasettings codedifficulty <easy|medium|hard>")
		}
		difficulty, err := parseDifficulty(ctx.Args[1])
		if err != nil {
			return NewErrorResponse(err.Error()), nil
		}
		settings, err := c.manager.UpdateCodeDifficulty(ctx.Channel, difficulty)
		if err != nil {
			return NewErrorResponse(err.Error()), nil
		}
		c.logAudit(ctx, "trivia_settings_code_difficulty", fmt.Sprintf("channel=%s difficulty=%s", ctx.Channel, difficulty))
		return NewResponse(formatSettingsMessage(ctx.Channel, settings)), nil
	case "points":
		if len(ctx.Args) != 4 {
			return nil, boterrors.NewInvalidSyntaxError("triviasettings", "!triviasettings points <base> <minimum> <hint_penalty>")
		}
		base, err := parseNonNegativeInt(ctx.Args[1])
		if err != nil {
			return NewErrorResponse(err.Error()), nil
		}
		minimum, err := parseNonNegativeInt(ctx.Args[2])
		if err != nil {
			return NewErrorResponse(err.Error()), nil
		}
		hintPenalty, err := parseNonNegativeInt(ctx.Args[3])
		if err != nil {
			return NewErrorResponse(err.Error()), nil
		}
		settings, err := c.manager.UpdatePoints(ctx.Channel, base, minimum, hintPenalty)
		if err != nil {
			return NewErrorResponse(err.Error()), nil
		}
		c.logAudit(ctx, "trivia_settings_points", fmt.Sprintf("channel=%s base=%d minimum=%d hint_penalty=%d", ctx.Channel, base, minimum, hintPenalty))
		return NewResponse(formatSettingsMessage(ctx.Channel, settings)), nil
	case "enabled":
		if len(ctx.Args) != 2 {
			return nil, boterrors.NewInvalidSyntaxError("triviasettings", "!triviasettings enabled on|off")
		}
		enabled, err := parseOnOff(ctx.Args[1])
		if err != nil {
			return NewErrorResponse(err.Error()), nil
		}
		settings, err := c.manager.UpdateEnabled(ctx.Channel, enabled)
		if err != nil {
			return NewErrorResponse(err.Error()), nil
		}
		c.logAudit(ctx, "trivia_settings_enabled", fmt.Sprintf("channel=%s enabled=%t", ctx.Channel, enabled))
		return NewResponse(formatSettingsMessage(ctx.Channel, settings)), nil
	default:
		return nil, boterrors.NewInvalidSyntaxError("triviasettings", "!triviasettings show|time|codetime|hint|difficulty|codedifficulty|points|enabled ...")
	}
}

func (c *TriviaSettingsCommand) logAudit(ctx *Context, action, details string) {
	if c.db == nil {
		return
	}
	if err := c.db.LogAuditAction(ctx.Nick, ctx.Hostmask, action, "", details, "success"); err != nil {
		fmt.Printf("Warning: failed to log audit action: %v\n", err)
	}
}

func (c *TriviaSettingsCommand) RequiredPermission() database.PermissionLevel {
	return database.LevelAdmin
}

func (c *TriviaSettingsCommand) Help() string {
	return "!triviasettings show|time|codetime|hint|difficulty|codedifficulty|points|enabled - Manage trivia/code channel settings (admin/owner)"
}

func (c *TriviaSettingsCommand) CooldownDuration() time.Duration {
	return 0
}

func formatSettingsMessage(channel string, settings trivia.ChannelSettings) string {
	return fmt.Sprintf(
		"Trivia settings for %s: enabled=%t, trivia_time=%ds, code_time=%ds, trivia_difficulty=%s, code_difficulty=%s, hints=%t, base_points=%d, minimum_points=%d, hint_penalty=%d",
		channel,
		settings.Enabled,
		settings.AnswerTimeSeconds,
		settings.CodeAnswerTimeSeconds,
		settings.Difficulty,
		settings.CodeDifficulty,
		settings.HintsEnabled,
		settings.BasePoints,
		settings.MinimumPoints,
		settings.HintPenalty,
	)
}

func parseDifficulty(input string) (string, error) {
	if !trivia.IsValidDifficulty(input) {
		return "", fmt.Errorf("difficulty must be easy, medium, or hard")
	}
	return trivia.NormalizeDifficulty(input), nil
}

func parseOnOff(input string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "on", "true", "enabled", "enable", "yes", "1":
		return true, nil
	case "off", "false", "disabled", "disable", "no", "0":
		return false, nil
	default:
		return false, fmt.Errorf("value must be on or off")
	}
}

func parseNonNegativeInt(input string) (int, error) {
	value, err := strconv.Atoi(input)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %s", input)
	}
	if value < 0 {
		return 0, fmt.Errorf("value must be non-negative")
	}
	return value, nil
}
