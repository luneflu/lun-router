package store

import (
	"os"
	"testing"
)

func TestModelStoreCustomModels(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "lun-modelstore-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ms, err := NewModelStore(tempDir)
	if err != nil {
		t.Fatalf("NewModelStore: %v", err)
	}

	// 1. Initial list empty
	list := ms.ListCustomModels("kilocode")
	if len(list) != 0 {
		t.Errorf("expected 0 custom models, got %d", len(list))
	}

	// 2. Add custom model
	cm := CustomModel{
		ID:                  "anthropic/claude-3.7-sonnet",
		ContextLength:       200000,
		MaxCompletionTokens: 8192,
	}
	if err := ms.AddCustomModel("kilocode", cm); err != nil {
		t.Fatalf("AddCustomModel: %v", err)
	}

	// 3. Verify HasCustomModel and List
	if !ms.HasCustomModel("kilocode", "anthropic/claude-3.7-sonnet") {
		t.Errorf("expected HasCustomModel to return true")
	}
	if !ms.HasCustomModel("kilocode", "kilocode/anthropic/claude-3.7-sonnet") {
		t.Errorf("expected HasCustomModel with prefix to return true")
	}

	list = ms.ListCustomModels("kilocode")
	if len(list) != 1 || list[0].ID != "anthropic/claude-3.7-sonnet" {
		t.Fatalf("expected 1 model with ID anthropic/claude-3.7-sonnet, got %+v", list)
	}

	// 4. Persistence check: new store reloads custom model
	ms2, err := NewModelStore(tempDir)
	if err != nil {
		t.Fatalf("NewModelStore reload: %v", err)
	}
	if !ms2.HasCustomModel("kilocode", "anthropic/claude-3.7-sonnet") {
		t.Errorf("expected reloaded store to have custom model")
	}

	// 5. Delete custom model
	if err := ms2.DeleteCustomModel("kilocode", "anthropic/claude-3.7-sonnet"); err != nil {
		t.Fatalf("DeleteCustomModel: %v", err)
	}
	if ms2.HasCustomModel("kilocode", "anthropic/claude-3.7-sonnet") {
		t.Errorf("expected custom model to be deleted")
	}
}
