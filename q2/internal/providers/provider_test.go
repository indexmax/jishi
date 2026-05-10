package providers

import "testing"

func TestNewQwenProviderDefaults(t *testing.T) {
	t.Setenv("DASHSCOPE_API_KEY", "test-key")
	t.Setenv("DASHSCOPE_BASE_URL", "")

	provider, err := New("qwen", Config{Model: "mock-agent", BaseURL: "https://api.openai.com/v1"})
	if err != nil {
		t.Fatal(err)
	}
	qwen, ok := provider.(*OpenAICompatible)
	if !ok {
		t.Fatalf("provider type=%T", provider)
	}
	if qwen.cfg.APIKey != "test-key" {
		t.Fatalf("api key=%q", qwen.cfg.APIKey)
	}
	if qwen.cfg.Model != "qwen-plus" {
		t.Fatalf("model=%q", qwen.cfg.Model)
	}
	if qwen.cfg.BaseURL != "https://dashscope.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("base url=%q", qwen.cfg.BaseURL)
	}
}

func TestNewQwenProviderRequiresDashScopeKey(t *testing.T) {
	t.Setenv("DASHSCOPE_API_KEY", "")
	if _, err := New("bailian", Config{}); err == nil {
		t.Fatal("expected missing DASHSCOPE_API_KEY error")
	}
}
