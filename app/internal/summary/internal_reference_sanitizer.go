package summary

import "regexp"

var inlineInternalReferencePattern = regexp.MustCompile(`(?:\[(?:m\d{3}|ref\d{3}|msg:\d+)\])+`)

func sanitizeSummaryInternalReferences(content string) string {
	if content == "" {
		return ""
	}
	return inlineInternalReferencePattern.ReplaceAllString(content, "")
}
