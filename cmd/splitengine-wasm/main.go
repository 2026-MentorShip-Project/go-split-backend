//go:build js && wasm

// Build with GOOS=js GOARCH=wasm go build -o engine.wasm ./cmd/splitengine-wasm.
package main

import (
	"encoding/json"
	"go-split-backend/internal/splitengine"
	"syscall/js"
)

func main() {
	split := js.FuncOf(func(_ js.Value, args []js.Value) any {
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
	})
	js.Global().Set("goSplitEngineVersion", splitengine.Version)
	js.Global().Set("goSplitDetail", split)
	select {}
}
