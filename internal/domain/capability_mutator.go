package domain

// ApplyCapabilityDelta returns a new ModelCapabilityProfile applying the empirical results.
func ApplyCapabilityDelta(base ModelCapabilityProfile, delta ModelCapabilityDelta) ModelCapabilityProfile {
	next := ModelCapabilityProfile{
		ModelID:          base.ModelID,
		ProviderID:       base.ProviderID,
		LastEvaluatedAt:  delta.EvaluatedAt,
		SchemaVersion:    base.SchemaVersion,
		SyntaxCompliance: cloneMap(base.SyntaxCompliance),
		SkillScores:      cloneMap(base.SkillScores),
		ObservedLimits:   cloneMapStr(base.ObservedLimits),
	}

	if next.SyntaxCompliance == nil {
		next.SyntaxCompliance = make(map[string]int)
	}
	if next.SkillScores == nil {
		next.SkillScores = make(map[string]int)
	}

	skillWeight := 10
	if !delta.Passed {
		skillWeight = -15
	}

	if delta.Format != "" {
		next.SyntaxCompliance[delta.Format] += skillWeight
		if next.SyntaxCompliance[delta.Format] < 0 {
			next.SyntaxCompliance[delta.Format] = 0
		}
	}

	if delta.Skill != "" {
		next.SkillScores[delta.Skill] += skillWeight
		if next.SkillScores[delta.Skill] < 0 {
			next.SkillScores[delta.Skill] = 0
		}
	}

	return next
}

func cloneMap(m map[string]int) map[string]int {
	if m == nil {
		return nil
	}
	c := make(map[string]int, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

func cloneMapStr(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	c := make(map[string]string, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}
