// Package matcher provides regex pattern matching for DNS domains.
package matcher

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// Matcher defines the interface for pattern matching.
type Matcher interface {
	// Match returns true if the domain matches any pattern.
	Match(domain string) bool
	// MatchingPattern returns the first matching pattern or empty string if none match.
	MatchingPattern(domain string) string
	// Patterns returns all configured patterns.
	Patterns() []string
}

// RegexMatcher implements Matcher using compiled regular expressions.
type RegexMatcher struct {
	patterns []*regexp.Regexp
	raw      []string
	mu       sync.RWMutex
}

// NewRegexMatcher creates a new RegexMatcher from a slice of regex pattern strings.
func NewRegexMatcher(patterns []string) (*RegexMatcher, error) {
	m := &RegexMatcher{
		patterns: make([]*regexp.Regexp, 0, len(patterns)),
		raw:      make([]string, 0, len(patterns)),
	}

	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		compiled, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern %q: %w", p, err)
		}

		m.patterns = append(m.patterns, compiled)
		m.raw = append(m.raw, p)
	}

	return m, nil
}

// Match returns true if the domain matches any of the patterns.
func (m *RegexMatcher) Match(domain string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Normalize domain (remove trailing dot if present)
	domain = strings.TrimSuffix(domain, ".")

	for _, p := range m.patterns {
		if p.MatchString(domain) {
			return true
		}
	}

	return false
}

// MatchingPattern returns the first pattern that matches the domain, or empty string if none match.
func (m *RegexMatcher) MatchingPattern(domain string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Normalize domain (remove trailing dot if present)
	domain = strings.TrimSuffix(domain, ".")

	for i, p := range m.patterns {
		if p.MatchString(domain) {
			return m.raw[i]
		}
	}

	return ""
}

// Patterns returns all configured patterns.
func (m *RegexMatcher) Patterns() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]string, len(m.raw))
	copy(result, m.raw)
	return result
}

// UpdatePatterns replaces all patterns with new ones.
func (m *RegexMatcher) UpdatePatterns(patterns []string) error {
	newPatterns := make([]*regexp.Regexp, 0, len(patterns))
	newRaw := make([]string, 0, len(patterns))

	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		compiled, err := regexp.Compile(p)
		if err != nil {
			return fmt.Errorf("invalid regex pattern %q: %w", p, err)
		}

		newPatterns = append(newPatterns, compiled)
		newRaw = append(newRaw, p)
	}

	m.mu.Lock()
	m.patterns = newPatterns
	m.raw = newRaw
	m.mu.Unlock()

	return nil
}

// NoOpMatcher is a matcher that never matches anything.
type NoOpMatcher struct{}

// NewNoOpMatcher creates a new NoOpMatcher.
func NewNoOpMatcher() *NoOpMatcher {
	return &NoOpMatcher{}
}

// Match always returns false.
func (m *NoOpMatcher) Match(domain string) bool {
	return false
}

// MatchingPattern always returns an empty string.
func (m *NoOpMatcher) MatchingPattern(domain string) string {
	return ""
}

// Patterns returns an empty slice.
func (m *NoOpMatcher) Patterns() []string {
	return []string{}
}
