package game

import (
	"strings"
	"sync"
)

// CaseMapping is an IRC server's advertised nickname comparison rule.
type CaseMapping string

const (
	CaseMappingASCII         CaseMapping = "ascii"
	CaseMappingStrictRFC1459 CaseMapping = "strict-rfc1459"
	CaseMappingRFC1459       CaseMapping = "rfc1459"
)

// ParseCaseMapping accepts only the three casemappings supported by the game
// identity boundary. Unknown values are rejected instead of guessed.
func ParseCaseMapping(value string) (CaseMapping, bool) {
	switch CaseMapping(strings.ToLower(value)) {
	case CaseMappingASCII:
		return CaseMappingASCII, true
	case CaseMappingStrictRFC1459:
		return CaseMappingStrictRFC1459, true
	case CaseMappingRFC1459:
		return CaseMappingRFC1459, true
	default:
		return "", false
	}
}

// DefaultCaseMapping uses the common RFC1459 mapping only for networks whose
// configured ids make that policy explicit. Unknown networks default to ASCII,
// which creates fewer accidental identity equivalences.
func DefaultCaseMapping(networkID string) CaseMapping {
	switch strings.ToLower(networkID) {
	case "libera", "libera-chat", "rizon":
		return CaseMappingRFC1459
	default:
		return CaseMappingASCII
	}
}

// IRCCasefold canonicalizes only IRC protocol ASCII bytes. It deliberately
// does not apply Unicode case conversion.
func IRCCasefold(value string, mapping CaseMapping) string {
	if _, ok := ParseCaseMapping(string(mapping)); !ok {
		mapping = CaseMappingASCII
	}
	result := []byte(value)
	for i, b := range result {
		if b >= 'A' && b <= 'Z' {
			result[i] = b + ('a' - 'A')
			continue
		}
		switch mapping {
		case CaseMappingRFC1459, CaseMappingStrictRFC1459:
			switch b {
			case '[':
				result[i] = '{'
			case ']':
				result[i] = '}'
			case '\\':
				result[i] = '|'
			case '^':
				if mapping == CaseMappingRFC1459 {
					result[i] = '~'
				}
			}
		}
	}
	return string(result)
}

// CaseMappingStore owns the live per-network CASEMAPPING learned from 005.
type CaseMappingStore struct {
	mu       sync.RWMutex
	mappings map[string]CaseMapping
}

func NewCaseMappingStore() *CaseMappingStore {
	return &CaseMappingStore{mappings: make(map[string]CaseMapping)}
}

func (s *CaseMappingStore) Get(networkID string) CaseMapping {
	if s == nil {
		return DefaultCaseMapping(networkID)
	}
	s.mu.RLock()
	mapping, ok := s.mappings[networkID]
	s.mu.RUnlock()
	if !ok {
		return DefaultCaseMapping(networkID)
	}
	return mapping
}

func (s *CaseMappingStore) Set(networkID string, mapping CaseMapping) bool {
	if s == nil {
		return false
	}
	parsed, ok := ParseCaseMapping(string(mapping))
	if !ok {
		return false
	}
	s.mu.Lock()
	s.mappings[networkID] = parsed
	s.mu.Unlock()
	return true
}

// UpdateFromISupport scans RPL_ISUPPORT parameters for CASEMAPPING. An absent
// or unsupported value leaves the current conservative mapping unchanged.
func (s *CaseMappingStore) UpdateFromISupport(networkID string, params []string) bool {
	for _, param := range params {
		for _, token := range strings.Fields(param) {
			key, value, found := strings.Cut(token, "=")
			if !found || !strings.EqualFold(key, "CASEMAPPING") {
				continue
			}
			mapping, ok := ParseCaseMapping(value)
			if !ok {
				return false
			}
			return s.Set(networkID, mapping)
		}
	}
	return false
}

func (s *CaseMappingStore) Fold(networkID, value string) string {
	return IRCCasefold(value, s.Get(networkID))
}
