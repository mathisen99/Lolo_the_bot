package trivia

import "errors"

var (
	ErrRoundAlreadyActive      = errors.New("trivia round already active")
	ErrNoActiveRound           = errors.New("no active trivia round")
	ErrHintAlreadyUsed         = errors.New("hint already used")
	ErrHintsDisabled           = errors.New("hints are disabled")
	ErrTriviaDisabled          = errors.New("trivia disabled")
	ErrTopicRequired           = errors.New("trivia topic required")
	ErrUnsupportedCodeLanguage = errors.New("unsupported code language")
	ErrGenerationFailed        = errors.New("failed to generate trivia question")
	ErrGeneratorDisabled       = errors.New("trivia generator unavailable")
)
