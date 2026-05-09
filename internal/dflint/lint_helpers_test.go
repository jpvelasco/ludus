package dflint

func builtinFindings(findings []Finding) []Finding {
	var result []Finding
	for _, f := range findings {
		if f.Source == "builtin" {
			result = append(result, f)
		}
	}
	return result
}
