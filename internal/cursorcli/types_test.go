package cursorcli

import "testing"

func TestParseAndClassify(t *testing.T) {
	initMsg, err := ParseLine([]byte(`{"type":"system","subtype":"init","model":"gpt-5.2"}`))
	if err != nil || !initMsg.IsSystemInit() || initMsg.Model != "gpt-5.2" {
		t.Fatalf("%+v %v", initMsg, err)
	}

	asst, err := ParseLine([]byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"Hi"}]}}`))
	if err != nil || !asst.IsAssistant() || asst.AssistantText() != "Hi" {
		t.Fatalf("%+v %v", asst, err)
	}

	tool, err := ParseLine([]byte(`{"type":"tool_call","subtype":"started"}`))
	if err != nil || !tool.IsToolCall() {
		t.Fatalf("%+v %v", tool, err)
	}

	res, err := ParseLine([]byte(`{"type":"result","result":"done","usage":{"inputTokens":10,"outputTokens":3,"cacheReadTokens":2,"cacheWriteTokens":1}}`))
	if err != nil || !res.IsResult() || res.Result != "done" {
		t.Fatalf("%+v %v", res, err)
	}
	if res.Usage == nil || res.Usage.InputTokens != 10 || res.Usage.OutputTokens != 3 || res.Usage.CacheReadTokens != 2 || res.Usage.CacheWriteTokens != 1 {
		t.Fatalf("usage %+v", res.Usage)
	}
}

func TestParseInvalid(t *testing.T) {
	if _, err := ParseLine([]byte("not-json")); err == nil {
		t.Fatal("expected error")
	}
}
