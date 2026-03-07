package ui

import "testing"

func TestCommandRegistryRegisterAndExecute(t *testing.T) {
	reg := NewCommandRegistry()
	called := false
	reg.Register(&Command{
		Name:        "test",
		Usage:       "/test",
		Category:    "Testing",
		Description: "A test command",
		Handler: func(args string) bool {
			called = true
			return true
		},
	})

	handled, exit := reg.Execute("/test")
	if !handled {
		t.Error("expected handled=true")
	}
	if exit {
		t.Error("expected exit=false")
	}
	if !called {
		t.Error("handler was not called")
	}
}

func TestCommandRegistryArgs(t *testing.T) {
	reg := NewCommandRegistry()
	var gotArgs string
	reg.Register(&Command{
		Name: "echo",
		Handler: func(args string) bool {
			gotArgs = args
			return true
		},
	})

	reg.Execute("/echo hello world")
	if gotArgs != "hello world" {
		t.Errorf("args = %q, want %q", gotArgs, "hello world")
	}
}

func TestCommandRegistryUnknown(t *testing.T) {
	reg := NewCommandRegistry()
	handled, _ := reg.Execute("/unknown")
	if handled {
		t.Error("unknown command should not be handled")
	}
}

func TestCommandRegistryNonSlash(t *testing.T) {
	reg := NewCommandRegistry()
	handled, _ := reg.Execute("hello")
	if handled {
		t.Error("non-slash input should not be handled")
	}
}

func TestCommandRegistryExit(t *testing.T) {
	reg := NewCommandRegistry()
	reg.Register(&Command{
		Name: "exit",
		Handler: func(args string) bool {
			return false // signal exit
		},
	})

	handled, exit := reg.Execute("/exit")
	if !handled {
		t.Error("expected handled=true")
	}
	if !exit {
		t.Error("expected exit=true")
	}
}

func TestCommandRegistryListByCategory(t *testing.T) {
	reg := NewCommandRegistry()
	reg.Register(&Command{Name: "a", Category: "Alpha"})
	reg.Register(&Command{Name: "b", Category: "Beta"})
	reg.Register(&Command{Name: "c", Category: "Alpha"})

	groups := reg.ListByCategory()
	if len(groups) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(groups))
	}
	if groups[0].Category != "Alpha" {
		t.Errorf("first category = %q, want Alpha", groups[0].Category)
	}
	if len(groups[0].Commands) != 2 {
		t.Errorf("Alpha commands = %d, want 2", len(groups[0].Commands))
	}
	if groups[1].Category != "Beta" {
		t.Errorf("second category = %q, want Beta", groups[1].Category)
	}
}

func TestCommandRegistryLookup(t *testing.T) {
	reg := NewCommandRegistry()
	reg.Register(&Command{Name: "find", Description: "find things"})

	cmd := reg.Lookup("find")
	if cmd == nil {
		t.Fatal("Lookup returned nil")
	}
	if cmd.Description != "find things" {
		t.Errorf("description = %q, want %q", cmd.Description, "find things")
	}

	if reg.Lookup("missing") != nil {
		t.Error("expected nil for missing command")
	}
}

func TestCommandRegistryNames(t *testing.T) {
	reg := NewCommandRegistry()
	reg.Register(&Command{Name: "zebra"})
	reg.Register(&Command{Name: "alpha"})

	names := reg.Names()
	if len(names) != 2 {
		t.Fatalf("names count = %d, want 2", len(names))
	}
	if names[0] != "alpha" || names[1] != "zebra" {
		t.Errorf("names = %v, want [alpha zebra]", names)
	}
}
