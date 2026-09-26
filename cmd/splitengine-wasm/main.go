//go:build js && wasm

// Build with GOOS=js GOARCH=wasm go build -o engine.wasm ./cmd/splitengine-wasm.
package main

import (
	"encoding/json"
	"errors"
	"syscall/js"

	"go-split-backend/internal/rulespec"
	"go-split-backend/internal/splitengine"
)

func main() {
	js.Global().Set("goSplitEngineVersion", splitengine.Version)
	js.Global().Set("goSplitDetail", js.FuncOf(splitDetail))
	js.Global().Set("goSplitValidateRule", js.FuncOf(validateRule))
	select {}
}

// splitDetail previews the allocation of one unsaved detail.
func splitDetail(_ js.Value, args []js.Value) any {
	if len(args) != 1 {
		return `{"error":"expected one JSON input"}`
	}
	var req struct {
		Detail  splitengine.Detail   `json:"detail"`
		Members []splitengine.Member `json:"members"`
		Rules   []splitengine.Rule   `json:"rules"`
		Order   []int64              `json:"split_order"`
	}
	if err := json.Unmarshal([]byte(args[0].String()), &req); err != nil {
		return `{"error":"invalid JSON input"}`
	}
	body, err := json.Marshal(splitengine.SplitDetail(req.Detail, req.Members, req.Rules, req.Order, 1))
	if err != nil {
		return `{"error":"invalid calculation input"}`
	}
	return string(body)
}

// ruleVerdict answers what saving this rule would do. A refused rule is an
// outcome rather than a failure, so it reports ok instead of error; error stays
// reserved for the bridge itself failing, which the wrapper turns into a throw.
type ruleVerdict struct {
	OK     bool            `json:"ok"`
	Code   rulespec.Code   `json:"code,omitempty"`
	Detail string          `json:"detail,omitempty"`
	Groups json.RawMessage `json:"groups,omitempty"`
	Rest   json.RawMessage `json:"rest,omitempty"`
}

// validateRule applies the checks a save applies, so a draft can be corrected
// without a round trip. The backend stays authoritative for saving.
func validateRule(_ js.Value, args []js.Value) any {
	if len(args) != 1 {
		return `{"error":"expected one JSON input"}`
	}
	var req struct {
		Groups   json.RawMessage `json:"groups"`
		Rest     json.RawMessage `json:"rest"`
		CondTags []string        `json:"cond_tags"`
	}
	if err := json.Unmarshal([]byte(args[0].String()), &req); err != nil {
		return `{"error":"invalid JSON input"}`
	}

	out := ruleVerdict{OK: true}
	groups, rest, err := rulespec.NormalizeRule(req.Groups, req.Rest, req.CondTags)
	switch {
	case err == nil:
		out.Groups, out.Rest = groups, rest
	default:
		var refused *rulespec.Error
		if !errors.As(err, &refused) {
			return `{"error":"invalid rule input"}`
		}
		out = ruleVerdict{Code: refused.Code, Detail: refused.Detail}
	}

	body, err := json.Marshal(out)
	if err != nil {
		return `{"error":"invalid rule input"}`
	}
	return string(body)
}
