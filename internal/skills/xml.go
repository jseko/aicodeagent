package skills

import (
	"html"
	"strings"
)

func ToPromptXML(skills []*Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<available_skills>\n")
	for _, skill := range skills {
		if skill == nil {
			continue
		}
		sb.WriteString("  <skill>\n")
		sb.WriteString("    <name>")
		sb.WriteString(escapeXML(skill.Name))
		sb.WriteString("</name>\n")
		sb.WriteString("    <description>")
		sb.WriteString(escapeXML(skill.Description))
		sb.WriteString("</description>\n")
		sb.WriteString("    <location>")
		sb.WriteString(escapeXML(skill.SkillFilePath()))
		sb.WriteString("</location>\n")
		sb.WriteString("  </skill>\n")
	}
	sb.WriteString("</available_skills>")
	return sb.String()
}

func escapeXML(value string) string {
	return html.EscapeString(value)
}
