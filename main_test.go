package main

import (
	"context"
	"errors"
	"os"
	"testing"

	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

func TestRun(t *testing.T) {
	var called bool
	err := run([]string{"-debug"}, func(_ context.Context, factory func() frameworkprovider.Provider, opts providerserver.ServeOpts) error {
		called = true
		if factory == nil {
			t.Fatal("expected provider factory")
		}
		if opts.Address != "registry.terraform.io/lucidsoftware/adldap" || !opts.Debug {
			t.Fatalf("unexpected serve opts: %+v", opts)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !called {
		t.Fatal("expected serve function to be called")
	}

	expectedErr := errors.New("boom")
	err = run(nil, func(_ context.Context, _ func() frameworkprovider.Provider, _ providerserver.ServeOpts) error {
		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected propagated error, got %v", err)
	}

	err = run([]string{"-unknown-flag"}, func(_ context.Context, _ func() frameworkprovider.Provider, _ providerserver.ServeOpts) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected invalid flag parsing to fail")
	}
}

func TestMainUsesServeProvider(t *testing.T) {
	originalServe := serveProvider
	originalFatal := fatalLog
	originalArgs := os.Args
	defer func() {
		serveProvider = originalServe
		fatalLog = originalFatal
		os.Args = originalArgs
	}()

	var called bool
	serveProvider = func(_ context.Context, _ func() frameworkprovider.Provider, _ providerserver.ServeOpts) error {
		called = true
		return nil
	}
	os.Args = []string{"terraform-provider-adldap", "-debug"}

	main()

	if !called {
		t.Fatal("expected main to invoke serveProvider")
	}
}

func TestMainFatalPath(t *testing.T) {
	originalServe := serveProvider
	originalFatal := fatalLog
	originalArgs := os.Args
	defer func() {
		serveProvider = originalServe
		fatalLog = originalFatal
		os.Args = originalArgs
	}()

	var fatalCalled bool
	serveProvider = func(_ context.Context, _ func() frameworkprovider.Provider, _ providerserver.ServeOpts) error {
		return errors.New("boom")
	}
	fatalLog = func(v ...any) {
		fatalCalled = true
	}
	os.Args = []string{"terraform-provider-adldap"}

	main()

	if !fatalCalled {
		t.Fatal("expected main to call fatalLog on serve error")
	}
}
