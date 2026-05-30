package summary

import (
	"regexp"
	"strings"
)

var summaryPrimaryHeadingPattern = regexp.MustCompile(`(?m)^##(?:\s|$)`)

func sanitizeSummaryOutput(content string) string {
	content = strings.TrimSpace(strings.ReplaceAll(content, "\r\n", "\n"))
	if content == "" {
		return ""
	}

	if index := firstSummaryHeadingIndex(content); index > 0 {
		content = strings.TrimSpace(content[index:])
	}

	return strings.TrimSpace(sanitizeSummaryInternalReferences(content))
}

func firstSummaryHeadingIndex(content string) int {
	match := summaryPrimaryHeadingPattern.FindStringIndex(content)
	if match == nil {
		return -1
	}
	return match[0]
}
