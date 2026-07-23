package main

import (
	"fmt"
	"motor-autonomo/internal/gatecampaign"
)

func main() {
	var m gatecampaign.SimplerFormatRecoveryCampaignManifest
	m.SchemaVersion = 1
	m.Name = "test"
	m.TimeoutSeconds = 90
	m.MaxCalls = 3
	m.InjectedFailures = 2
	m.MaxOutputTokens = 192
	m.ProbePrompt = "foo"
	m.Bindings = []gatecampaign.RuntimeGateBinding{
		{Provider: "foo", BindingID: "foo", BaseURL: "foo", Model: "foo", APIKeyEnvironment: "foo", MaxOutputField: "foo"},
	}
	err := m.Validate()
	fmt.Println(err)
}
