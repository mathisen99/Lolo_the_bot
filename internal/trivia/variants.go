package trivia

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

type VariantFamily string

const (
	VariantFamilyFact     VariantFamily = "fact"
	VariantFamilyChoice   VariantFamily = "choice"
	VariantFamilyNumeric  VariantFamily = "numeric"
	VariantFamilyOrdering VariantFamily = "ordering"
	VariantFamilyFillIn   VariantFamily = "fillin"
	VariantFamilySource   VariantFamily = "source"
	VariantFamilySequence VariantFamily = "sequence"
)

type JudgeMode string

const (
	JudgeModeSemantic       JudgeMode = "semantic"
	JudgeModeChoice         JudgeMode = "choice"
	JudgeModeNumericClosest JudgeMode = "numeric_closest"
	JudgeModeOrderingExact  JudgeMode = "ordering_exact"
	JudgeModeExactOnly      JudgeMode = "exact_only"
)

type VariantSpec struct {
	Name               string
	Label              string
	Family             VariantFamily
	Weight             int
	Live               bool
	SupportsSpeed      bool
	JudgeMode          JudgeMode
	MetadataSchemaJSON string
	PromptRules        string
	ValidateGenerated  func(*GeneratedQuestion) error
	BuildAliases       func([]string, TriviaQuestionMetadata) ([]string, error)
	FormatQuestion     func(*StoredQuestion) (string, error)
	UseHint            func(*activeRound) (string, bool, error)
	ResolveTimeout     func(*activeRound, []GuessLog) (*judgedWinner, string, bool, error)
	ParseGuess         func(*activeRound, string) bool
}

type PyramidMetadata struct {
	PyramidClues []string `json:"pyramid_clues"`
}

type ConnectionMetadata struct {
	ConnectionClues []string `json:"connection_clues"`
}

type RealFakeMetadata struct {
	Choices []TriviaChoice `json:"choices"`
}

type XWordMetadata struct {
	XWordPattern string `json:"xword_pattern"`
}

type LabeledChoice struct {
	Label string `json:"label"`
	Text  string `json:"text"`
}

type BinaryChoiceMetadata struct {
	Prompt  string          `json:"prompt,omitempty"`
	Choices []LabeledChoice `json:"choices"`
}

type ChronologyMetadata struct {
	Events []LabeledChoice `json:"events"`
}

type ClosestNumberMetadata struct {
	Unit         string `json:"unit,omitempty"`
	AllowDecimal bool   `json:"allow_decimal,omitempty"`
}

type QuoteSourceMetadata struct {
	SourceType string `json:"source_type,omitempty"`
}

type SequenceMetadata struct {
	SequenceItems   []string `json:"sequence_items"`
	MissingPosition int      `json:"missing_position,omitempty"`
}

type AcronymMetadata struct {
	Acronym  string `json:"acronym"`
	Category string `json:"category,omitempty"`
}

type TitleCompletionMetadata struct {
	TitleTemplate string `json:"title_template"`
}

type CategoryLockMetadata struct {
	RequiredCategory string `json:"required_category"`
}

var triviaVariantSpecList = []*VariantSpec{
	{
		Name:               VariantClassic,
		Label:              "Trivia",
		Family:             VariantFamilyFact,
		Weight:             16,
		Live:               true,
		SupportsSpeed:      true,
		JudgeMode:          JudgeModeSemantic,
		MetadataSchemaJSON: `{}`,
		PromptRules: `- Ask one concise, objective trivia question.
- answer must be a single factual solution or a small set of exact equivalent answers.`,
		ValidateGenerated: normalizeClassicGeneratedQuestion,
	},
	{
		Name:               VariantPyramid,
		Label:              "Pyramid",
		Family:             VariantFamilyFact,
		Weight:             7,
		Live:               true,
		SupportsSpeed:      false,
		JudgeMode:          JudgeModeSemantic,
		MetadataSchemaJSON: `{"pyramid_clues":["string","string","string"]}`,
		PromptRules: `- metadata.pyramid_clues must contain exactly 3 clues from vaguest to most specific.
- question should be the opening clue text or a short intro for the clue sequence.
- answer must be the single thing identified by all clues.`,
		ValidateGenerated: normalizePyramidGeneratedQuestion,
		FormatQuestion:    formatPyramidQuestion,
		UseHint:           usePyramidHint,
	},
	{
		Name:               VariantConnection,
		Label:              "Connection",
		Family:             VariantFamilyFact,
		Weight:             7,
		Live:               true,
		SupportsSpeed:      true,
		JudgeMode:          JudgeModeSemantic,
		MetadataSchemaJSON: `{"connection_clues":["string","string","string"]}`,
		PromptRules: `- metadata.connection_clues must contain exactly 3 concise clues.
- question should ask what links the clues.
- answer must be the one concept that connects all three clues.`,
		ValidateGenerated: normalizeConnectionGeneratedQuestion,
		FormatQuestion:    formatConnectionQuestion,
	},
	{
		Name:               VariantRealFake,
		Label:              "Real/Fake",
		Family:             VariantFamilyChoice,
		Weight:             6,
		Live:               true,
		SupportsSpeed:      true,
		JudgeMode:          JudgeModeChoice,
		MetadataSchemaJSON: `{"choices":[{"label":"A","text":"string","is_true":true},{"label":"B","text":"string","is_true":false},{"label":"C","text":"string","is_true":true}]}`,
		PromptRules: `- metadata.choices must contain exactly 3 statements with exactly 1 false statement.
- question should ask which statement is fake.
- answer must identify the false statement.`,
		ValidateGenerated: normalizeRealFakeGeneratedQuestion,
		BuildAliases:      buildRealFakeAliases,
		FormatQuestion:    formatRealFakeQuestion,
	},
	{
		Name:               VariantClosestYear,
		Label:              "Closest Year",
		Family:             VariantFamilyNumeric,
		Weight:             5,
		Live:               true,
		SupportsSpeed:      false,
		JudgeMode:          JudgeModeNumericClosest,
		MetadataSchemaJSON: `{}`,
		PromptRules: `- question must ask for one specific year.
- answer must be the exact integer year only.
- avoid ambiguous date ranges or eras.`,
		ValidateGenerated: normalizeClosestYearGeneratedQuestion,
		ResolveTimeout:    resolveClosestYearTimeout,
		ParseGuess:        isExactClosestYearGuess,
	},
	{
		Name:               VariantXWord,
		Label:              "XWord",
		Family:             VariantFamilyFillIn,
		Weight:             5,
		Live:               true,
		SupportsSpeed:      false,
		JudgeMode:          JudgeModeSemantic,
		MetadataSchemaJSON: `{"xword_pattern":"string"}`,
		PromptRules: `- question must be a clue for the missing word or phrase.
- metadata.xword_pattern must contain the visible fill-in pattern.
- answer must exactly fit the pattern.`,
		ValidateGenerated: normalizeXWordGeneratedQuestion,
		FormatQuestion:    formatXWordQuestion,
	},
	{
		Name:               VariantChronology,
		Label:              "Chronology",
		Family:             VariantFamilyOrdering,
		Weight:             6,
		Live:               true,
		SupportsSpeed:      false,
		JudgeMode:          JudgeModeOrderingExact,
		MetadataSchemaJSON: `{"events":[{"label":"A","text":"string"},{"label":"B","text":"string"},{"label":"C","text":"string"}]}`,
		PromptRules: `- metadata.events must contain 3 or 4 events.
- question should ask for oldest-to-newest chronological order.
- answer must be the label order, like "B-A-C" or "2-1-3".`,
		ValidateGenerated: normalizeChronologyGeneratedQuestion,
		BuildAliases:      buildChronologyAliases,
		FormatQuestion:    formatChronologyQuestion,
	},
	{
		Name:               VariantClosestNum,
		Label:              "Closest Number",
		Family:             VariantFamilyNumeric,
		Weight:             5,
		Live:               true,
		SupportsSpeed:      false,
		JudgeMode:          JudgeModeNumericClosest,
		MetadataSchemaJSON: `{"unit":"string","allow_decimal":false}`,
		PromptRules: `- question must ask for one specific numeric answer.
- metadata.unit may describe the unit.
- metadata.allow_decimal controls whether decimal answers are allowed.
- answer must be the exact numeric value only.`,
		ValidateGenerated: normalizeClosestNumberGeneratedQuestion,
		ResolveTimeout:    resolveClosestNumberTimeout,
		ParseGuess:        isExactClosestNumberGuess,
	},
	{
		Name:               VariantHigherLower,
		Label:              "Higher/Lower",
		Family:             VariantFamilyChoice,
		Weight:             5,
		Live:               true,
		SupportsSpeed:      true,
		JudgeMode:          JudgeModeChoice,
		MetadataSchemaJSON: `{"prompt":"string","choices":[{"label":"A","text":"string"},{"label":"B","text":"string"}]}`,
		PromptRules: `- metadata.prompt should state the comparison, like which item is higher, larger, or earlier.
- metadata.choices must contain exactly 2 labeled options.
- answer must identify the correct option with text or label.`,
		ValidateGenerated: normalizeHigherLowerGeneratedQuestion,
		BuildAliases:      buildHigherLowerAliases,
		FormatQuestion:    formatBinaryChoiceQuestion,
	},
	{
		Name:               VariantOddOneOut,
		Label:              "Odd One Out",
		Family:             VariantFamilyChoice,
		Weight:             5,
		Live:               true,
		SupportsSpeed:      true,
		JudgeMode:          JudgeModeChoice,
		MetadataSchemaJSON: `{"choices":[{"label":"A","text":"string"},{"label":"B","text":"string"},{"label":"C","text":"string"}]}`,
		PromptRules: `- metadata.choices must contain 3 or 4 labeled options.
- question should ask which option is the odd one out.
- answer must identify the odd option with text or label.`,
		ValidateGenerated: normalizeOddOneOutGeneratedQuestion,
		BuildAliases:      buildOddOneOutAliases,
		FormatQuestion:    formatChoiceListQuestion,
	},
	{
		Name:               VariantQuoteSource,
		Label:              "Quote Source",
		Family:             VariantFamilySource,
		Weight:             5,
		Live:               true,
		SupportsSpeed:      true,
		JudgeMode:          JudgeModeSemantic,
		MetadataSchemaJSON: `{"source_type":"person|book|movie|game|show"}`,
		PromptRules: `- question should quote or paraphrase a recognizable line and ask for its source.
- metadata.source_type is optional and should be one of person, book, movie, game, or show when known.
- answer must identify the exact source.`,
		ValidateGenerated: normalizeQuoteSourceGeneratedQuestion,
	},
	{
		Name:               VariantSequence,
		Label:              "Sequence",
		Family:             VariantFamilySequence,
		Weight:             5,
		Live:               true,
		SupportsSpeed:      true,
		JudgeMode:          JudgeModeSemantic,
		MetadataSchemaJSON: `{"sequence_items":["string","string","string"],"missing_position":0}`,
		PromptRules: `- metadata.sequence_items must contain at least 3 visible sequence entries.
- metadata.missing_position is optional and may indicate a missing slot.
- question should ask for the missing or next item in the sequence.`,
		ValidateGenerated: normalizeSequenceGeneratedQuestion,
		FormatQuestion:    formatSequenceQuestion,
	},
	{
		Name:               VariantAcronym,
		Label:              "Acronym",
		Family:             VariantFamilyFact,
		Weight:             4,
		Live:               true,
		SupportsSpeed:      true,
		JudgeMode:          JudgeModeSemantic,
		MetadataSchemaJSON: `{"acronym":"string","category":"string"}`,
		PromptRules: `- metadata.acronym must contain the acronym to expand.
- metadata.category is optional context such as computing, science, or medicine.
- question should ask for the expansion of the acronym.`,
		ValidateGenerated: normalizeAcronymGeneratedQuestion,
		FormatQuestion:    formatAcronymQuestion,
	},
	{
		Name:               VariantTitleComp,
		Label:              "Title Completion",
		Family:             VariantFamilyFillIn,
		Weight:             4,
		Live:               true,
		SupportsSpeed:      true,
		JudgeMode:          JudgeModeSemantic,
		MetadataSchemaJSON: `{"title_template":"string"}`,
		PromptRules: `- metadata.title_template must contain the visible title with blanks.
- question should ask for the missing word or phrase in the title.
- answer must be the missing word or phrase only.`,
		ValidateGenerated: normalizeTitleCompletionGeneratedQuestion,
		FormatQuestion:    formatTitleCompletionQuestion,
	},
	{
		Name:               VariantCategory,
		Label:              "Category Lock",
		Family:             VariantFamilyFact,
		Weight:             3,
		Live:               true,
		SupportsSpeed:      true,
		JudgeMode:          JudgeModeSemantic,
		MetadataSchemaJSON: `{"required_category":"string"}`,
		PromptRules: `- metadata.required_category must name the required answer category.
- question should clearly stay inside that category.
- answer must be a normal trivia answer that fits the category exactly.`,
		ValidateGenerated: normalizeCategoryLockGeneratedQuestion,
		FormatQuestion:    formatCategoryLockQuestion,
	},
	{
		Name:               VariantDefDuel,
		Label:              "Definition Duel",
		Family:             VariantFamilyChoice,
		Weight:             2,
		Live:               true,
		SupportsSpeed:      true,
		JudgeMode:          JudgeModeChoice,
		MetadataSchemaJSON: `{"choices":[{"label":"A","text":"string"},{"label":"B","text":"string"}]}`,
		PromptRules: `- metadata.choices must contain exactly 2 labeled candidate answers.
- question should be the definition or clue text.
- answer must identify the correct candidate with text or label.`,
		ValidateGenerated: normalizeDefinitionDuelGeneratedQuestion,
		BuildAliases:      buildDefinitionDuelAliases,
		FormatQuestion:    formatChoiceListQuestion,
	},
}

var triviaVariantSpecs = func() map[string]*VariantSpec {
	out := make(map[string]*VariantSpec, len(triviaVariantSpecList))
	for _, spec := range triviaVariantSpecList {
		out[spec.Name] = spec
	}
	return out
}()

func stableTriviaVariantNames() []string {
	names := make([]string, 0, len(triviaVariantSpecList))
	for _, spec := range triviaVariantSpecList {
		if spec.Live {
			names = append(names, spec.Name)
		}
	}
	return names
}

func stableTriviaVariantSpecs() []*VariantSpec {
	specs := make([]*VariantSpec, 0, len(triviaVariantSpecList))
	for _, spec := range triviaVariantSpecList {
		if spec.Live {
			specs = append(specs, spec)
		}
	}
	return specs
}

func triviaVariantSpec(variant string) *VariantSpec {
	if spec, ok := triviaVariantSpecs[NormalizeTriviaVariant(variant)]; ok {
		return spec
	}
	return triviaVariantSpecs[VariantClassic]
}

func chooseTriviaVariant(recent []string) string {
	specs := stableTriviaVariantSpecs()
	if len(specs) == 0 {
		return VariantClassic
	}

	lastVariant := ""
	if len(recent) > 0 {
		lastVariant = NormalizeTriviaVariant(recent[0])
	}

	blockedFamily := VariantFamily("")
	if len(recent) >= 2 {
		first := triviaVariantSpec(recent[0]).Family
		second := triviaVariantSpec(recent[1]).Family
		if first != "" && first == second {
			blockedFamily = first
		}
	}

	options := filterVariantOptions(specs, lastVariant, blockedFamily)
	if len(options) == 0 {
		options = filterVariantOptions(specs, lastVariant, "")
	}
	if len(options) == 0 {
		options = specs
	}

	totalWeight := 0
	for _, spec := range options {
		if spec.Weight > 0 {
			totalWeight += spec.Weight
		}
	}
	if totalWeight <= 0 {
		return VariantClassic
	}

	roll := rand.New(rand.NewSource(time.Now().UnixNano())).Intn(totalWeight)
	for _, spec := range options {
		if roll < spec.Weight {
			return spec.Name
		}
		roll -= spec.Weight
	}
	return options[len(options)-1].Name
}

func filterVariantOptions(specs []*VariantSpec, lastVariant string, blockedFamily VariantFamily) []*VariantSpec {
	options := make([]*VariantSpec, 0, len(specs))
	for _, spec := range specs {
		if spec.Weight <= 0 {
			continue
		}
		if lastVariant != "" && spec.Name == lastVariant && len(specs) > 1 {
			continue
		}
		if blockedFamily != "" && spec.Family == blockedFamily {
			continue
		}
		options = append(options, spec)
	}
	return options
}

func normalizeGeneratedTriviaQuestion(question *GeneratedQuestion) error {
	if question == nil {
		return fmt.Errorf("invalid trivia payload: empty")
	}

	question.Variant = NormalizeTriviaVariant(question.Variant)
	question.Question = strings.TrimSpace(question.Question)
	question.Answer = strings.TrimSpace(question.Answer)
	question.Hint = strings.TrimSpace(question.Hint)
	question.UniquenessKey = strings.TrimSpace(question.UniquenessKey)

	metadata, err := normalizeTriviaMetadata(question.Metadata)
	if err != nil {
		return err
	}
	question.Metadata = metadata
	question.Aliases = sanitizeTriviaAliases(question.Aliases)

	spec := triviaVariantSpec(question.Variant)
	if spec.ValidateGenerated == nil {
		return fmt.Errorf("invalid trivia payload: no validator for variant %q", question.Variant)
	}
	if err := spec.ValidateGenerated(question); err != nil {
		return err
	}

	question.Aliases = sanitizeTriviaAliases(question.Aliases)

	if question.Question == "" {
		return fmt.Errorf("invalid trivia payload: question is empty")
	}
	if question.Answer == "" {
		return fmt.Errorf("invalid trivia payload: answer is empty")
	}
	if len(question.Answer) > maxAnswerLength {
		return fmt.Errorf("invalid trivia payload: answer exceeds %d chars", maxAnswerLength)
	}
	if question.Hint == "" {
		return fmt.Errorf("invalid trivia payload: hint is empty")
	}
	if question.UniquenessKey != "" && strings.ContainsAny(question.UniquenessKey, "\n\r") {
		return fmt.Errorf("invalid trivia payload: uniqueness_key must be one line")
	}

	return nil
}

func normalizeClassicGeneratedQuestion(question *GeneratedQuestion) error {
	metadata, err := normalizeEmptyTriviaMetadata(question.Metadata, VariantClassic)
	if err != nil {
		return err
	}
	question.Metadata = metadata
	return nil
}

func normalizePyramidGeneratedQuestion(question *GeneratedQuestion) error {
	meta, err := parseTriviaMetadata[PyramidMetadata](question.Metadata)
	if err != nil {
		return fmt.Errorf("invalid trivia payload: pyramid metadata: %w", err)
	}
	meta.PyramidClues = normalizeMetadataStrings(meta.PyramidClues)
	if len(meta.PyramidClues) != 3 {
		return fmt.Errorf("invalid trivia payload: pyramid_clues must contain exactly 3 clues")
	}
	if question.Question == "" {
		question.Question = meta.PyramidClues[0]
	}
	if question.Hint == "" {
		question.Hint = meta.PyramidClues[1]
	}
	question.Metadata, err = marshalTriviaMetadata(meta)
	return err
}

func normalizeConnectionGeneratedQuestion(question *GeneratedQuestion) error {
	meta, err := parseTriviaMetadata[ConnectionMetadata](question.Metadata)
	if err != nil {
		return fmt.Errorf("invalid trivia payload: connection metadata: %w", err)
	}
	meta.ConnectionClues = normalizeMetadataStrings(meta.ConnectionClues)
	if len(meta.ConnectionClues) != 3 {
		return fmt.Errorf("invalid trivia payload: connection_clues must contain exactly 3 clues")
	}
	if question.Question == "" {
		question.Question = "What connects these clues?"
	}
	question.Metadata, err = marshalTriviaMetadata(meta)
	return err
}

func normalizeRealFakeGeneratedQuestion(question *GeneratedQuestion) error {
	meta, err := parseTriviaMetadata[RealFakeMetadata](question.Metadata)
	if err != nil {
		return fmt.Errorf("invalid trivia payload: real_fake metadata: %w", err)
	}
	choices, falseChoice, falseIndex, err := normalizeRealFakeChoices(meta.Choices)
	if err != nil {
		return err
	}
	meta.Choices = choices
	if question.Question == "" {
		question.Question = "Which statement is fake?"
	}
	if !matchesRealFakeAnswer(question.Answer, falseChoice, falseIndex) {
		return fmt.Errorf("invalid trivia payload: answer must identify the false real_fake choice")
	}
	question.Answer = falseChoice.Text
	question.Aliases = append(question.Aliases, falseChoice.Label, strconv.Itoa(falseIndex+1))
	question.Metadata, err = marshalTriviaMetadata(meta)
	return err
}

func normalizeClosestYearGeneratedQuestion(question *GeneratedQuestion) error {
	metadata, err := normalizeEmptyTriviaMetadata(question.Metadata, VariantClosestYear)
	if err != nil {
		return err
	}
	question.Metadata = metadata
	year, ok := extractYearGuess(question.Answer)
	if !ok {
		return fmt.Errorf("invalid trivia payload: closest_year answer must be a year")
	}
	question.Answer = strconv.Itoa(year)
	return nil
}

func normalizeXWordGeneratedQuestion(question *GeneratedQuestion) error {
	meta, err := parseTriviaMetadata[XWordMetadata](question.Metadata)
	if err != nil {
		return fmt.Errorf("invalid trivia payload: xword metadata: %w", err)
	}
	meta.XWordPattern = strings.TrimSpace(meta.XWordPattern)
	if meta.XWordPattern == "" {
		return fmt.Errorf("invalid trivia payload: xword_pattern is empty")
	}
	question.Metadata, err = marshalTriviaMetadata(meta)
	return err
}

func normalizeChronologyGeneratedQuestion(question *GeneratedQuestion) error {
	meta, err := parseTriviaMetadata[ChronologyMetadata](question.Metadata)
	if err != nil {
		return fmt.Errorf("invalid trivia payload: chronology metadata: %w", err)
	}
	choices, err := normalizeLabeledChoices(meta.Events, 3, 4)
	if err != nil {
		return fmt.Errorf("invalid trivia payload: chronology events: %w", err)
	}
	meta.Events = choices
	order, numericOrder, err := normalizeChronologyAnswer(question.Answer, len(meta.Events))
	if err != nil {
		return err
	}
	if question.Question == "" {
		question.Question = "Put these events in chronological order from oldest to newest."
	}
	question.Answer = order
	question.Aliases = append(question.Aliases, numericOrder)
	question.Metadata, err = marshalTriviaMetadata(meta)
	return err
}

func normalizeClosestNumberGeneratedQuestion(question *GeneratedQuestion) error {
	meta, err := parseTriviaMetadata[ClosestNumberMetadata](question.Metadata)
	if err != nil {
		return fmt.Errorf("invalid trivia payload: closest_number metadata: %w", err)
	}
	meta.Unit = strings.TrimSpace(meta.Unit)
	value, ok := extractNumericGuess(question.Answer, meta.AllowDecimal)
	if !ok {
		return fmt.Errorf("invalid trivia payload: closest_number answer must be numeric")
	}
	if !meta.AllowDecimal {
		question.Answer = strconv.Itoa(int(math.Round(value)))
	} else {
		question.Answer = formatFloat(value)
	}
	question.Metadata, err = marshalTriviaMetadata(meta)
	return err
}

func normalizeHigherLowerGeneratedQuestion(question *GeneratedQuestion) error {
	meta, err := parseTriviaMetadata[BinaryChoiceMetadata](question.Metadata)
	if err != nil {
		return fmt.Errorf("invalid trivia payload: higher_lower metadata: %w", err)
	}
	meta.Prompt = strings.TrimSpace(meta.Prompt)
	meta.Choices, err = normalizeLabeledChoices(meta.Choices, 2, 2)
	if err != nil {
		return fmt.Errorf("invalid trivia payload: higher_lower choices: %w", err)
	}
	if question.Question == "" {
		question.Question = meta.Prompt
	}
	correct, index, ok := matchChoiceAnswer(question.Answer, meta.Choices)
	if !ok {
		return fmt.Errorf("invalid trivia payload: higher_lower answer must identify one of the choices")
	}
	question.Answer = correct.Text
	question.Aliases = append(question.Aliases, correct.Label, strconv.Itoa(index+1))
	if index == 0 {
		question.Aliases = append(question.Aliases, "left")
	} else {
		question.Aliases = append(question.Aliases, "right")
	}
	question.Metadata, err = marshalTriviaMetadata(meta)
	return err
}

func normalizeOddOneOutGeneratedQuestion(question *GeneratedQuestion) error {
	meta, err := parseTriviaMetadata[ChoiceListMetadata](question.Metadata)
	if err != nil {
		return fmt.Errorf("invalid trivia payload: odd_one_out metadata: %w", err)
	}
	meta.Choices, err = normalizeLabeledChoices(meta.Choices, 3, 4)
	if err != nil {
		return fmt.Errorf("invalid trivia payload: odd_one_out choices: %w", err)
	}
	if question.Question == "" {
		question.Question = "Which option is the odd one out?"
	}
	correct, index, ok := matchChoiceAnswer(question.Answer, meta.Choices)
	if !ok {
		return fmt.Errorf("invalid trivia payload: odd_one_out answer must identify one of the choices")
	}
	question.Answer = correct.Text
	question.Aliases = append(question.Aliases, correct.Label, strconv.Itoa(index+1))
	question.Metadata, err = marshalTriviaMetadata(meta)
	return err
}

func normalizeQuoteSourceGeneratedQuestion(question *GeneratedQuestion) error {
	meta, err := parseTriviaMetadata[QuoteSourceMetadata](question.Metadata)
	if err != nil {
		return fmt.Errorf("invalid trivia payload: quote_source metadata: %w", err)
	}
	meta.SourceType = strings.TrimSpace(strings.ToLower(meta.SourceType))
	switch meta.SourceType {
	case "", "person", "book", "movie", "game", "show":
	default:
		return fmt.Errorf("invalid trivia payload: source_type must be one of person, book, movie, game, show")
	}
	question.Metadata, err = marshalTriviaMetadata(meta)
	return err
}

func normalizeSequenceGeneratedQuestion(question *GeneratedQuestion) error {
	meta, err := parseTriviaMetadata[SequenceMetadata](question.Metadata)
	if err != nil {
		return fmt.Errorf("invalid trivia payload: sequence metadata: %w", err)
	}
	meta.SequenceItems = normalizeMetadataStrings(meta.SequenceItems)
	if len(meta.SequenceItems) < 3 {
		return fmt.Errorf("invalid trivia payload: sequence_items must contain at least 3 items")
	}
	if meta.MissingPosition < 0 || meta.MissingPosition > len(meta.SequenceItems)+1 {
		return fmt.Errorf("invalid trivia payload: missing_position out of range")
	}
	if question.Question == "" {
		question.Question = "What comes next in this sequence?"
	}
	question.Metadata, err = marshalTriviaMetadata(meta)
	return err
}

func normalizeAcronymGeneratedQuestion(question *GeneratedQuestion) error {
	meta, err := parseTriviaMetadata[AcronymMetadata](question.Metadata)
	if err != nil {
		return fmt.Errorf("invalid trivia payload: acronym metadata: %w", err)
	}
	meta.Acronym = strings.TrimSpace(meta.Acronym)
	meta.Category = strings.TrimSpace(meta.Category)
	if meta.Acronym == "" {
		return fmt.Errorf("invalid trivia payload: acronym is empty")
	}
	if question.Question == "" {
		if meta.Category != "" {
			question.Question = fmt.Sprintf("What does %s stand for in %s?", meta.Acronym, meta.Category)
		} else {
			question.Question = fmt.Sprintf("What does %s stand for?", meta.Acronym)
		}
	}
	question.Metadata, err = marshalTriviaMetadata(meta)
	return err
}

func normalizeTitleCompletionGeneratedQuestion(question *GeneratedQuestion) error {
	meta, err := parseTriviaMetadata[TitleCompletionMetadata](question.Metadata)
	if err != nil {
		return fmt.Errorf("invalid trivia payload: title_completion metadata: %w", err)
	}
	meta.TitleTemplate = strings.TrimSpace(meta.TitleTemplate)
	if meta.TitleTemplate == "" {
		return fmt.Errorf("invalid trivia payload: title_template is empty")
	}
	if question.Question == "" {
		question.Question = "Fill in the missing word or phrase in this title."
	}
	question.Metadata, err = marshalTriviaMetadata(meta)
	return err
}

func normalizeCategoryLockGeneratedQuestion(question *GeneratedQuestion) error {
	meta, err := parseTriviaMetadata[CategoryLockMetadata](question.Metadata)
	if err != nil {
		return fmt.Errorf("invalid trivia payload: category_lock metadata: %w", err)
	}
	meta.RequiredCategory = strings.TrimSpace(meta.RequiredCategory)
	if meta.RequiredCategory == "" {
		return fmt.Errorf("invalid trivia payload: required_category is empty")
	}
	question.Metadata, err = marshalTriviaMetadata(meta)
	return err
}

func normalizeDefinitionDuelGeneratedQuestion(question *GeneratedQuestion) error {
	meta, err := parseTriviaMetadata[BinaryChoiceMetadata](question.Metadata)
	if err != nil {
		return fmt.Errorf("invalid trivia payload: definition_duel metadata: %w", err)
	}
	meta.Prompt = strings.TrimSpace(meta.Prompt)
	meta.Choices, err = normalizeLabeledChoices(meta.Choices, 2, 2)
	if err != nil {
		return fmt.Errorf("invalid trivia payload: definition_duel choices: %w", err)
	}
	correct, index, ok := matchChoiceAnswer(question.Answer, meta.Choices)
	if !ok {
		return fmt.Errorf("invalid trivia payload: definition_duel answer must identify one of the choices")
	}
	question.Answer = correct.Text
	question.Aliases = append(question.Aliases, correct.Label, strconv.Itoa(index+1))
	question.Metadata, err = marshalTriviaMetadata(meta)
	return err
}

func normalizeTriviaMetadata(metadata TriviaQuestionMetadata) (TriviaQuestionMetadata, error) {
	trimmed := bytes.TrimSpace(metadata)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return emptyTriviaMetadata(), nil
	}
	if !json.Valid(trimmed) {
		return nil, fmt.Errorf("invalid trivia payload: metadata is not valid JSON")
	}
	return TriviaQuestionMetadata(append([]byte(nil), trimmed...)), nil
}

func normalizeEmptyTriviaMetadata(metadata TriviaQuestionMetadata, variant string) (TriviaQuestionMetadata, error) {
	type emptyMetadata struct{}
	if _, err := parseTriviaMetadata[emptyMetadata](metadata); err != nil {
		return nil, fmt.Errorf("invalid trivia payload: %s metadata must be empty", variant)
	}
	return emptyTriviaMetadata(), nil
}

func emptyTriviaMetadata() TriviaQuestionMetadata {
	return TriviaQuestionMetadata([]byte("{}"))
}

func cloneTriviaMetadata(metadata TriviaQuestionMetadata) TriviaQuestionMetadata {
	if len(metadata) == 0 {
		return emptyTriviaMetadata()
	}
	return TriviaQuestionMetadata(append([]byte(nil), metadata...))
}

func parseTriviaMetadata[T any](metadata TriviaQuestionMetadata) (T, error) {
	var target T
	normalized, err := normalizeTriviaMetadata(metadata)
	if err != nil {
		return target, err
	}
	if err := strictJSONUnmarshal([]byte(normalized), &target); err != nil {
		return target, err
	}
	return target, nil
}

func marshalTriviaMetadata(value any) (TriviaQuestionMetadata, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return normalizeTriviaMetadata(payload)
}

func sanitizeTriviaAliases(aliases []string) []string {
	validAliases := make([]string, 0, len(aliases))
	seenAliases := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		trimmed := strings.TrimSpace(alias)
		if trimmed == "" || len(trimmed) > maxAliasLength {
			continue
		}
		key := NormalizeAnswer(trimmed)
		if key == "" {
			continue
		}
		if _, exists := seenAliases[key]; exists {
			continue
		}
		seenAliases[key] = struct{}{}
		validAliases = append(validAliases, trimmed)
	}
	return validAliases
}

func normalizeRealFakeChoices(choices []TriviaChoice) ([]TriviaChoice, TriviaChoice, int, error) {
	if len(choices) != 3 {
		return nil, TriviaChoice{}, 0, fmt.Errorf("invalid trivia payload: real_fake choices must contain exactly 3 items")
	}

	falseIndex := -1
	normalized := make([]TriviaChoice, 0, len(choices))
	for idx, choice := range choices {
		text := strings.TrimSpace(choice.Text)
		if text == "" {
			return nil, TriviaChoice{}, 0, fmt.Errorf("invalid trivia payload: real_fake choice text is empty")
		}
		label := string(rune('A' + idx))
		normalized = append(normalized, TriviaChoice{
			Label:  label,
			Text:   text,
			IsTrue: choice.IsTrue,
		})
		if !choice.IsTrue {
			if falseIndex >= 0 {
				return nil, TriviaChoice{}, 0, fmt.Errorf("invalid trivia payload: real_fake must contain exactly one false choice")
			}
			falseIndex = idx
		}
	}
	if falseIndex < 0 {
		return nil, TriviaChoice{}, 0, fmt.Errorf("invalid trivia payload: real_fake must contain exactly one false choice")
	}
	return normalized, normalized[falseIndex], falseIndex, nil
}

func normalizeLabeledChoices(choices []LabeledChoice, minCount, maxCount int) ([]LabeledChoice, error) {
	if len(choices) < minCount || len(choices) > maxCount {
		return nil, fmt.Errorf("must contain between %d and %d choices", minCount, maxCount)
	}
	normalized := make([]LabeledChoice, 0, len(choices))
	for idx, choice := range choices {
		text := strings.TrimSpace(choice.Text)
		if text == "" {
			return nil, fmt.Errorf("choice text is empty")
		}
		normalized = append(normalized, LabeledChoice{
			Label: string(rune('A' + idx)),
			Text:  text,
		})
	}
	return normalized, nil
}

func normalizeMetadataStrings(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

func matchChoiceAnswer(answer string, choices []LabeledChoice) (LabeledChoice, int, bool) {
	normalized := NormalizeAnswer(answer)
	if normalized == "" {
		return LabeledChoice{}, 0, false
	}
	for idx, choice := range choices {
		if normalized == NormalizeAnswer(choice.Text) || normalized == NormalizeAnswer(choice.Label) || normalized == NormalizeAnswer(strconv.Itoa(idx+1)) {
			return choice, idx, true
		}
	}
	return LabeledChoice{}, 0, false
}

func matchesRealFakeAnswer(answer string, falseChoice TriviaChoice, falseIndex int) bool {
	normalized := NormalizeAnswer(answer)
	if normalized == "" {
		return false
	}
	return normalized == NormalizeAnswer(falseChoice.Text) ||
		normalized == NormalizeAnswer(falseChoice.Label) ||
		normalized == NormalizeAnswer(strconv.Itoa(falseIndex+1))
}

func normalizeChronologyAnswer(answer string, count int) (string, string, error) {
	parts := strings.FieldsFunc(strings.ToUpper(strings.TrimSpace(answer)), func(r rune) bool {
		switch {
		case r >= 'A' && r <= 'Z':
			return false
		case r >= '0' && r <= '9':
			return false
		default:
			return true
		}
	})
	if len(parts) != count {
		return "", "", fmt.Errorf("invalid trivia payload: chronology answer must list %d labels", count)
	}
	seenLabels := make(map[string]struct{}, count)
	seenNumbers := make(map[int]struct{}, count)
	labelOrder := make([]string, 0, count)
	numberOrder := make([]string, 0, count)
	for _, part := range parts {
		if len(part) == 1 && part[0] >= 'A' && part[0] < byte('A'+count) {
			if _, ok := seenLabels[part]; ok {
				return "", "", fmt.Errorf("invalid trivia payload: chronology answer repeats %s", part)
			}
			seenLabels[part] = struct{}{}
			labelOrder = append(labelOrder, part)
			numberOrder = append(numberOrder, strconv.Itoa(int(part[0]-'A'+1)))
			continue
		}
		number, err := strconv.Atoi(part)
		if err != nil || number < 1 || number > count {
			return "", "", fmt.Errorf("invalid trivia payload: chronology answer item %q is invalid", part)
		}
		if _, ok := seenNumbers[number]; ok {
			return "", "", fmt.Errorf("invalid trivia payload: chronology answer repeats %d", number)
		}
		seenNumbers[number] = struct{}{}
		labelOrder = append(labelOrder, string(rune('A'+number-1)))
		numberOrder = append(numberOrder, strconv.Itoa(number))
	}
	return strings.Join(labelOrder, "-"), strings.Join(numberOrder, "-"), nil
}

func buildTriviaAliases(variant string, aliases []string, metadata TriviaQuestionMetadata) ([]string, error) {
	spec := triviaVariantSpec(variant)
	if spec.BuildAliases == nil {
		return sanitizeTriviaAliases(aliases), nil
	}
	return spec.BuildAliases(aliases, metadata)
}

func buildRealFakeAliases(aliases []string, metadata TriviaQuestionMetadata) ([]string, error) {
	meta, err := parseTriviaMetadata[RealFakeMetadata](metadata)
	if err != nil {
		return nil, err
	}
	extra := append([]string(nil), aliases...)
	for idx, choice := range meta.Choices {
		if choice.IsTrue {
			continue
		}
		extra = append(extra, choice.Text, choice.Label, strconv.Itoa(idx+1))
	}
	return sanitizeTriviaAliases(extra), nil
}

func buildChronologyAliases(aliases []string, metadata TriviaQuestionMetadata) ([]string, error) {
	return sanitizeTriviaAliases(aliases), nil
}

func buildHigherLowerAliases(aliases []string, metadata TriviaQuestionMetadata) ([]string, error) {
	return sanitizeTriviaAliases(aliases), nil
}

func buildOddOneOutAliases(aliases []string, metadata TriviaQuestionMetadata) ([]string, error) {
	return sanitizeTriviaAliases(aliases), nil
}

func buildDefinitionDuelAliases(aliases []string, metadata TriviaQuestionMetadata) ([]string, error) {
	return sanitizeTriviaAliases(aliases), nil
}

func formatTriviaQuestionForDisplay(question *StoredQuestion) (string, error) {
	spec := triviaVariantSpec(question.Variant)
	if spec.FormatQuestion == nil {
		return question.Question, nil
	}
	return spec.FormatQuestion(question)
}

func formatPyramidQuestion(question *StoredQuestion) (string, error) {
	meta, err := parseTriviaMetadata[PyramidMetadata](question.Metadata)
	if err != nil {
		return "", err
	}
	clue := question.Question
	if len(meta.PyramidClues) > 0 {
		clue = meta.PyramidClues[0]
	}
	return fmt.Sprintf("Clue 1/3: %s", clue), nil
}

func formatConnectionQuestion(question *StoredQuestion) (string, error) {
	meta, err := parseTriviaMetadata[ConnectionMetadata](question.Metadata)
	if err != nil {
		return "", err
	}
	clues := strings.Join(meta.ConnectionClues, " | ")
	if strings.TrimSpace(question.Question) == "" {
		return fmt.Sprintf("What connects these clues? %s", clues), nil
	}
	return fmt.Sprintf("%s %s", question.Question, clues), nil
}

func formatRealFakeQuestion(question *StoredQuestion) (string, error) {
	meta, err := parseTriviaMetadata[RealFakeMetadata](question.Metadata)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(meta.Choices))
	for _, choice := range meta.Choices {
		parts = append(parts, fmt.Sprintf("%s) %s", choice.Label, choice.Text))
	}
	if strings.TrimSpace(question.Question) == "" {
		return strings.Join(parts, " | "), nil
	}
	return fmt.Sprintf("%s %s", question.Question, strings.Join(parts, " | ")), nil
}

func formatXWordQuestion(question *StoredQuestion) (string, error) {
	meta, err := parseTriviaMetadata[XWordMetadata](question.Metadata)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s [%s]", question.Question, meta.XWordPattern), nil
}

func formatChronologyQuestion(question *StoredQuestion) (string, error) {
	meta, err := parseTriviaMetadata[ChronologyMetadata](question.Metadata)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s %s", question.Question, formatLabeledChoices(meta.Events)), nil
}

func formatBinaryChoiceQuestion(question *StoredQuestion) (string, error) {
	meta, err := parseTriviaMetadata[BinaryChoiceMetadata](question.Metadata)
	if err != nil {
		return "", err
	}
	prompt := question.Question
	if prompt == "" {
		prompt = meta.Prompt
	}
	if prompt == "" {
		prompt = "Choose the correct option."
	}
	return fmt.Sprintf("%s %s", prompt, formatLabeledChoices(meta.Choices)), nil
}

type ChoiceListMetadata struct {
	Choices []LabeledChoice `json:"choices"`
}

func formatChoiceListQuestion(question *StoredQuestion) (string, error) {
	meta, err := parseTriviaMetadata[ChoiceListMetadata](question.Metadata)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(question.Question) == "" {
		return formatLabeledChoices(meta.Choices), nil
	}
	return fmt.Sprintf("%s %s", question.Question, formatLabeledChoices(meta.Choices)), nil
}

func formatSequenceQuestion(question *StoredQuestion) (string, error) {
	meta, err := parseTriviaMetadata[SequenceMetadata](question.Metadata)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s %s", question.Question, strings.Join(meta.SequenceItems, " -> ")), nil
}

func formatAcronymQuestion(question *StoredQuestion) (string, error) {
	meta, err := parseTriviaMetadata[AcronymMetadata](question.Metadata)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s [%s]", question.Question, meta.Acronym), nil
}

func formatTitleCompletionQuestion(question *StoredQuestion) (string, error) {
	meta, err := parseTriviaMetadata[TitleCompletionMetadata](question.Metadata)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s [%s]", question.Question, meta.TitleTemplate), nil
}

func formatCategoryLockQuestion(question *StoredQuestion) (string, error) {
	meta, err := parseTriviaMetadata[CategoryLockMetadata](question.Metadata)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s [category: %s]", question.Question, meta.RequiredCategory), nil
}

func formatLabeledChoices(choices []LabeledChoice) string {
	parts := make([]string, 0, len(choices))
	for _, choice := range choices {
		parts = append(parts, fmt.Sprintf("%s) %s", choice.Label, choice.Text))
	}
	return strings.Join(parts, " | ")
}

func triviaVariantLabel(variant string) string {
	return triviaVariantSpec(variant).Label
}

func triviaVariantUsesSemanticJudge(variant string) bool {
	return variantJudgeMode(variant) == JudgeModeSemantic
}

func variantJudgeMode(variant string) JudgeMode {
	return triviaVariantSpec(variant).JudgeMode
}

func variantSupportsSpeed(variant string) bool {
	return triviaVariantSpec(variant).SupportsSpeed
}

func selectRoundModifiers(variant string) []string {
	modifiers := make([]string, 0, 1)
	if !variantSupportsSpeed(variant) {
		return modifiers
	}
	if rand.New(rand.NewSource(time.Now().UnixNano())).Intn(100) < 20 {
		modifiers = append(modifiers, ModifierSpeed)
	}
	return modifiers
}

func useVariantHint(round *activeRound) (string, error) {
	spec := triviaVariantSpec(round.Variant)
	if spec.UseHint != nil {
		if hint, handled, err := spec.UseHint(round); handled || err != nil {
			return hint, err
		}
	}
	return round.Hint, nil
}

func usePyramidHint(round *activeRound) (string, bool, error) {
	meta, err := parseTriviaMetadata[PyramidMetadata](round.Metadata)
	if err != nil {
		return "", true, err
	}
	if nextIndex := round.RevealedClues; nextIndex < len(meta.PyramidClues) {
		round.RevealedClues = nextIndex + 1
		return fmt.Sprintf("Pyramid clue %d/3: %s", nextIndex+1, meta.PyramidClues[nextIndex]), true, nil
	}
	return round.Hint, true, nil
}

func resolveVariantTimeout(round *activeRound, guesses []GuessLog) (*judgedWinner, string, bool, error) {
	spec := triviaVariantSpec(round.Variant)
	if spec.ResolveTimeout == nil {
		return nil, "", false, nil
	}
	return spec.ResolveTimeout(round, guesses)
}

func resolveClosestYearTimeout(round *activeRound, guesses []GuessLog) (*judgedWinner, string, bool, error) {
	winner := findClosestYearWinner(round, guesses)
	if winner == nil {
		return nil, "", true, nil
	}
	msg := fmt.Sprintf(
		"Closest year wins: %s's guess %q was closest. Official year: %s",
		winner.Nick,
		winner.Message,
		round.Answer,
	)
	return winner, msg, true, nil
}

func resolveClosestNumberTimeout(round *activeRound, guesses []GuessLog) (*judgedWinner, string, bool, error) {
	winner := findClosestNumberWinner(round, guesses)
	if winner == nil {
		return nil, "", true, nil
	}
	msg := fmt.Sprintf(
		"Closest number wins: %s's guess %q was closest. Official number: %s",
		winner.Nick,
		winner.Message,
		round.Answer,
	)
	return winner, msg, true, nil
}

func isCorrectTriviaGuess(round *activeRound, message string) bool {
	spec := triviaVariantSpec(round.Variant)
	if spec.ParseGuess != nil {
		return spec.ParseGuess(round, message)
	}
	normalized := NormalizeAnswer(message)
	if normalized == "" {
		return false
	}
	_, ok := round.AcceptedAnswers[normalized]
	return ok
}

func isExactClosestYearGuess(round *activeRound, message string) bool {
	guessYear, ok := extractYearGuess(message)
	if !ok {
		return false
	}
	answerYear, ok := extractYearGuess(round.Answer)
	return ok && guessYear == answerYear
}

func isExactClosestNumberGuess(round *activeRound, message string) bool {
	meta, err := parseTriviaMetadata[ClosestNumberMetadata](round.Metadata)
	if err != nil {
		return false
	}
	guess, ok := extractNumericGuess(message, meta.AllowDecimal)
	if !ok {
		return false
	}
	answer, ok := extractNumericGuess(round.Answer, meta.AllowDecimal)
	if !ok {
		return false
	}
	return numbersEqual(guess, answer, meta.AllowDecimal)
}

func extractYearGuess(input string) (int, bool) {
	value, ok := extractNumericGuess(input, false)
	if !ok {
		return 0, false
	}
	return int(math.Round(value)), true
}

func extractNumericGuess(input string, allowDecimal bool) (float64, bool) {
	tokens := strings.Fields(strings.TrimSpace(input))
	if len(tokens) == 0 {
		return 0, false
	}

	found := false
	var value float64
	for _, token := range tokens {
		clean := strings.Trim(token, ".,!?;:'\"()[]{}")
		if clean == "" {
			continue
		}
		if !strings.ContainsAny(clean, "0123456789") {
			continue
		}
		if !allowDecimal && strings.Contains(clean, ".") {
			continue
		}

		var parsed float64
		var err error
		if allowDecimal {
			parsed, err = strconv.ParseFloat(clean, 64)
		} else {
			var integer int
			integer, err = strconv.Atoi(clean)
			parsed = float64(integer)
		}
		if err != nil {
			continue
		}
		if found {
			return 0, false
		}
		found = true
		value = parsed
	}
	return value, found
}

func numbersEqual(a, b float64, allowDecimal bool) bool {
	if !allowDecimal {
		return int(math.Round(a)) == int(math.Round(b))
	}
	return math.Abs(a-b) < 1e-9
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func findClosestNumberWinner(round *activeRound, guesses []GuessLog) *judgedWinner {
	meta, err := parseTriviaMetadata[ClosestNumberMetadata](round.Metadata)
	if err != nil {
		return nil
	}
	answerValue, ok := extractNumericGuess(round.Answer, meta.AllowDecimal)
	if !ok {
		return nil
	}

	var best *judgedWinner
	bestDistance := math.MaxFloat64
	for _, guess := range guesses {
		guessValue, ok := extractNumericGuess(guess.Message, meta.AllowDecimal)
		if !ok {
			continue
		}
		distance := math.Abs(guessValue - answerValue)
		if best == nil || distance < bestDistance || (math.Abs(distance-bestDistance) < 1e-9 && guess.Timestamp.Before(best.Timestamp)) {
			bestDistance = distance
			best = &judgedWinner{
				GuessLog:   guess,
				ID:         guess.ID,
				Nick:       guess.Nick,
				Message:    guess.Message,
				Timestamp:  guess.Timestamp,
				Confidence: 1,
			}
		}
	}
	return best
}

func streakBonusForPreviousWins(previousWins int) int {
	switch {
	case previousWins >= 3:
		return 3
	case previousWins == 2:
		return 2
	case previousWins == 1:
		return 1
	default:
		return 0
	}
}

func hasModifier(modifiers []string, target string) bool {
	for _, modifier := range modifiers {
		if modifier == target {
			return true
		}
	}
	return false
}
