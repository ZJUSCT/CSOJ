package devpods

import (
	"strings"

	"github.com/ZJUSCT/CSOJ/internal/database/models"
)

// NormalizeAllowedTags trims template tag restrictions, removes empty values,
// and preserves the first occurrence of each tag.
func NormalizeAllowedTags(tags models.StringArray) models.StringArray {
	normalized := make(models.StringArray, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
	}
	return normalized
}

// TemplateAllowedForUser reports whether a user's comma-separated tags match
// at least one of the template's allowed tags. An empty restriction is public.
func TemplateAllowedForUser(userTags string, allowedTags models.StringArray) bool {
	allowedTags = NormalizeAllowedTags(allowedTags)
	if len(allowedTags) == 0 {
		return true
	}

	userTagSet := make(map[string]struct{})
	for _, tag := range strings.Split(userTags, ",") {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			userTagSet[tag] = struct{}{}
		}
	}
	for _, tag := range allowedTags {
		if _, ok := userTagSet[tag]; ok {
			return true
		}
	}
	return false
}
