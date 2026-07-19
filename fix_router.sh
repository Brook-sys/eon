#!/bin/bash
sed -i -e 's/return domain.SelectModelBinding(candidates, requiredTokens, now)/profiles := make(map[string]domain.ModelCapabilityProfile)\n\treq := domain.RequiredCapability{}\n\treturn domain.SelectSkilledModelBinding(candidates, profiles, req, requiredTokens, now)/g' internal/kernel/model_router.go
