package cors

import "strings"

type OriginRule struct {
	AllowAll bool
	Prefix   string
	Suffix   string
	Exact    string
}

func ParseOriginRules(env string) []OriginRule {
	if env == "" {
		return nil
	}
	var rules []OriginRule
	for _, o := range strings.Split(env, ",") {
		if o = strings.TrimSpace(o); o != "" {
			if o == "*" {
				rules = append(rules, OriginRule{AllowAll: true})
			} else if strings.Contains(o, "*") {
				parts := strings.SplitN(o, "*", 2)
				rules = append(rules, OriginRule{Prefix: parts[0], Suffix: parts[1]})
			} else {
				rules = append(rules, OriginRule{Exact: o})
			}
		}
	}
	return rules
}

func NewOriginValidator(rules []OriginRule) func(string) bool {
	return func(origin string) bool {
		for _, rule := range rules {
			if rule.AllowAll {
				return true
			}
			if rule.Exact != "" && origin == rule.Exact {
				return true
			}
			if (rule.Prefix != "" || rule.Suffix != "") && strings.HasPrefix(origin, rule.Prefix) && strings.HasSuffix(origin, rule.Suffix) {
				return true
			}
		}
		return false
	}
}
