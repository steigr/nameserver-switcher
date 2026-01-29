package matcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegexMatcher(t *testing.T) {
	tests := []struct {
		name        string
		patterns    []string
		expectError bool
	}{
		{
			name:        "empty patterns",
			patterns:    []string{},
			expectError: false,
		},
		{
			name:        "valid patterns",
			patterns:    []string{`.*\.example\.com$`, `^test\.`, `.*\.org$`},
			expectError: false,
		},
		{
			name:        "invalid regex",
			patterns:    []string{"[invalid"},
			expectError: true,
		},
		{
			name:        "patterns with whitespace",
			patterns:    []string{`  .*\.example\.com$  `, ""},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := NewRegexMatcher(tt.patterns)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, m)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, m)
			}
		})
	}
}

func TestRegexMatcher_Match(t *testing.T) {
	patterns := []string{
		`.*\.example\.com$`,
		`^test\.`,
		`.*\.cdn\.provider\.net$`,
	}

	m, err := NewRegexMatcher(patterns)
	require.NoError(t, err)

	tests := []struct {
		domain   string
		expected bool
	}{
		{"www.example.com", true},
		{"api.example.com", true},
		{"example.com", false},
		{"test.anything.org", true},
		{"test.example.com", true},
		{"nottest.example.com", true},
		{"www.cdn.provider.net", true},
		{"random.domain.io", false},
		{"www.example.com.", true},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			result := m.Match(tt.domain)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRegexMatcher_MatchingPattern(t *testing.T) {
	patterns := []string{
		`.*\.example\.com$`,
		`^test\.`,
	}

	m, err := NewRegexMatcher(patterns)
	require.NoError(t, err)

	tests := []struct {
		domain   string
		expected string
	}{
		{"www.example.com", `.*\.example\.com$`},
		{"test.anything.org", `^test\.`},
		{"random.domain.io", ""},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			result := m.MatchingPattern(tt.domain)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRegexMatcher_Patterns(t *testing.T) {
	patterns := []string{`.*\.example\.com$`, `^test\.`}

	m, err := NewRegexMatcher(patterns)
	require.NoError(t, err)

	result := m.Patterns()
	assert.Equal(t, patterns, result)
}

func TestRegexMatcher_UpdatePatterns(t *testing.T) {
	m, err := NewRegexMatcher([]string{`.*\.example\.com$`})
	require.NoError(t, err)

	assert.True(t, m.Match("www.example.com"))
	assert.False(t, m.Match("www.test.org"))

	err = m.UpdatePatterns([]string{`.*\.test\.org$`})
	require.NoError(t, err)

	assert.False(t, m.Match("www.example.com"))
	assert.True(t, m.Match("www.test.org"))
}

func TestRegexMatcher_UpdatePatterns_Invalid(t *testing.T) {
	m, err := NewRegexMatcher([]string{`.*\.example\.com$`})
	require.NoError(t, err)

	err = m.UpdatePatterns([]string{"[invalid"})
	assert.Error(t, err)

	assert.True(t, m.Match("www.example.com"))
}

func TestRegexMatcher_UpdatePatterns_WithEmptyPatterns(t *testing.T) {
	m, err := NewRegexMatcher([]string{`.*\.example\.com$`})
	require.NoError(t, err)

	// Update with patterns that include empty strings
	err = m.UpdatePatterns([]string{"", "  ", `.*\.test\.org$`, ""})
	require.NoError(t, err)

	// Only valid pattern should be kept
	assert.Equal(t, []string{`.*\.test\.org$`}, m.Patterns())
	assert.True(t, m.Match("www.test.org"))
	assert.False(t, m.Match("www.example.com"))
}

func TestNoOpMatcher(t *testing.T) {
	m := NewNoOpMatcher()

	assert.False(t, m.Match("anything.example.com"))
	assert.Equal(t, "", m.MatchingPattern("anything.example.com"))
	assert.Empty(t, m.Patterns())
}
