package larking

import (
	"testing"

	"google.golang.org/genproto/googleapis/api/annotations"
)

func TestRuleSelector(t *testing.T) {
	rule := &annotations.HttpRule{
		Selector: "larking.LarkingService.Get",
		Pattern: &annotations.HttpRule_Get{
			Get: "/v1/{name=projects/*/instances/*}",
		},
	}
	healthzRule := &annotations.HttpRule{
		Selector: "grpc.health.v1.Health.Check",
		Pattern: &annotations.HttpRule_Get{
			Get: "/healthz",
		},
	}
	wildcardRule := &annotations.HttpRule{
		Selector: "wildcard.Service.*",
		Pattern: &annotations.HttpRule_Get{
			Get: "/wildcard",
		},
	}

	var hr ruleSelector
	hr.setRules([]*annotations.HttpRule{rule, healthzRule, wildcardRule})

	t.Log(&hr)

	rules := hr.getRules("larking.LarkingService.Get")
	if rules == nil {
		t.Fatal("got nil")
	}
	got := rules[0]
	if got != rule {
		t.Fatalf("got %v, want %v", got, rule)
	}

	rules = hr.getRules("grpc.health.v1.Health.Check")
	if rules == nil {
		t.Fatal("got nil")
	}
	got = rules[0]
	if got != healthzRule {
		t.Fatalf("got %v, want %v", got, healthzRule)
	}

	rules = hr.getRules("wildcard.Service.Get.DeepMethod")
	if rules == nil {
		t.Fatal("got nil")
	}
	got = rules[0]
	if got != wildcardRule {
		t.Fatalf("got %v, want %v", got, wildcardRule)
	}
}
