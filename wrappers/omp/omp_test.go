// SPDX-License-Identifier: MIT

package omp

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	sessionkit "github.com/antst/sessionbus/bus/sdk/go"
)

func TestOMPHelloAndLaneArguments(t *testing.T) {
	wrapper := New("/tmp/daemon.sock", "provisional", NativeExecutable{}, "/tmp/extension.mjs")
	hello, err := wrapper.Hello(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if hello.Product != Product || !hello.SupportsMessageRun ||
		!slices.Equal(hello.SupportedOpenFields, []string{"cwd", "permission_mode", "model", "reasoning_effort", "arguments"}) ||
		len(hello.ExtraArguments) != 2 {
		t.Fatalf("hello = %+v", hello)
	}
	arguments, err := ompLaneArguments(sessionkit.OpenOptions{
		PermissionMode: "default", Model: "provider/model", ReasoningEffort: "auto",
		Arguments: []string{"--system-prompt", "--not-a-flag", "--append-system-prompt=tail"},
	}, "native-session")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"--session", "native-session", "--model", "provider/model", "--thinking", "auto",
		"--system-prompt", "--not-a-flag", "--append-system-prompt=tail",
	}
	if !slices.Equal(arguments, want) {
		t.Fatalf("lane arguments = %#v, want %#v", arguments, want)
	}
}

func TestOMPLaneArgumentsRejectTypedOwnerConflicts(t *testing.T) {
	for _, test := range []struct {
		name string
		open sessionkit.OpenOptions
	}{
		{"permission", sessionkit.OpenOptions{PermissionMode: "yolo"}},
		{"model", sessionkit.OpenOptions{Model: " bad"}},
		{"thinking", sessionkit.OpenOptions{ReasoningEffort: "ultra"}},
		{"topology", sessionkit.OpenOptions{Arguments: []string{"--mode", "print"}}},
		{"extension", sessionkit.OpenOptions{Arguments: []string{"--extension", "/tmp/other.mjs"}}},
		{"resume-value", sessionkit.OpenOptions{Arguments: []string{"--resume", "native-session"}}},
		{"approval", sessionkit.OpenOptions{Arguments: []string{"--approval-mode", "yolo"}}},
		{"unknown", sessionkit.OpenOptions{Arguments: []string{"--verbose"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ompLaneArguments(test.open, ""); err == nil {
				t.Fatal("invalid lane arguments were accepted")
			}
		})
	}
}

func TestOMPWrapperOpenResumeAndCloseUsesNativeOwner(t *testing.T) {
	options, _ := nativeOwnerFixture(t, "")
	wrapper := New(options.DaemonSocket, options.Provisional, options.Native, options.Extension)
	wrapper.SetCaller(options.PrimaryCaller)
	result, err := wrapper.Open(nativeOwnerTestContext(t), sessionkit.OpenRequest{
		Name: "requested@local", Groups: []string{"fixture"}, ResumeSessionID: ompNativeOwnerSID,
		Open: sessionkit.OpenOptions{Cwd: options.CWD},
	})
	if err != nil || result.SessionID != ompNativeOwnerSID {
		t.Fatalf("Open = %+v, %v", result, err)
	}
	if wrapper.binding.OwnerToken != ompNativeOwnerToken || wrapper.binding.SessionID != ompNativeOwnerSID || !wrapper.opened {
		t.Fatalf("binding = %+v, opened %v", wrapper.binding, wrapper.opened)
	}
	if err = wrapper.Close(nativeOwnerTestContext(t), sessionkit.SessionCloseRequest{Forget: true}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-wrapper.owner.Done():
	default:
		t.Fatal("Close returned before NativeOwner joined")
	}
	if _, err = wrapper.Open(context.Background(), sessionkit.OpenRequest{Name: "again@local"}); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("reused Open = %v", err)
	}
}

func TestOMPWrapperOpenFreshConfirmsNativeNameAndClose(t *testing.T) {
	options, _ := nativeOwnerFixture(t, "")
	wrapper := New(options.DaemonSocket, options.Provisional, options.Native, options.Extension)
	wrapper.SetCaller(options.PrimaryCaller)
	result, err := wrapper.Open(nativeOwnerTestContext(t), sessionkit.OpenRequest{
		Name: "fresh-requested@local", Groups: []string{"fixture"},
		Open: sessionkit.OpenOptions{Cwd: options.CWD},
	})
	if err != nil || result.SessionID != ompNativeOwnerSID {
		t.Fatalf("fresh Open = %+v, %v", result, err)
	}
	if !wrapper.opened || wrapper.binding.SessionID != ompNativeOwnerSID {
		t.Fatalf("fresh binding = %+v, opened %v", wrapper.binding, wrapper.opened)
	}
	if err = wrapper.Close(nativeOwnerTestContext(t), sessionkit.SessionCloseRequest{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-wrapper.owner.Done():
	default:
		t.Fatal("fresh Close returned before NativeOwner joined")
	}
}

func TestOMPNameAndResumeValidation(t *testing.T) {
	if _, err := ompNamePart("missing-domain"); err == nil {
		t.Fatal("invalid lane name accepted")
	}
	if _, err := ompLaneArguments(sessionkit.OpenOptions{}, "bad session"); err == nil {
		t.Fatal("invalid resume identity accepted")
	}
}

func TestOMPOpenBindingAdoptsOnlyOmittedResumeCWD(t *testing.T) {
	binding := OwnerBinding{
		Scope: ownerScopePrimary, Mode: ownerModeRPC,
		SessionID: "resume-session", CWD: "/persisted/project",
	}
	if err := validateOMPOpenBinding(binding, "/launch/project", "resume-session", false); err != nil {
		t.Fatalf("omitted-cwd resume = %v", err)
	}
	if err := validateOMPOpenBinding(binding, "/launch/project", "resume-session", true); err == nil {
		t.Fatal("explicit-cwd resume mismatch was accepted")
	}
	if err := validateOMPOpenBinding(binding, "/launch/project", "", false); err == nil {
		t.Fatal("fresh cwd mismatch was accepted")
	}
}

func TestOMPAdoptOpenRejectsCallerCancellationAtCommitBoundary(t *testing.T) {
	wrapper := &Wrapper{}
	wrapper.ctx, wrapper.cancel = context.WithCancelCause(context.Background())
	t.Cleanup(func() { wrapper.cancel(errNativeOwnerClosed) })
	caller, cancelCaller := context.WithCancel(context.Background())
	wrapper.mu.Lock()
	result := make(chan error, 1)
	go func() {
		result <- wrapper.adoptOpen(caller, context.Background(), OwnerBinding{SessionID: "native-session"})
	}()
	cancelCaller()
	wrapper.mu.Unlock()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("adopt Open = %v", err)
	}
	if wrapper.opened {
		t.Fatal("canceled Open was committed")
	}
}

func TestOMPAdoptOpenRejectsClosingWithoutAContextError(t *testing.T) {
	wrapper := &Wrapper{closing: true}
	wrapper.ctx, wrapper.cancel = context.WithCancelCause(context.Background())
	t.Cleanup(func() { wrapper.cancel(errNativeOwnerClosed) })
	err := wrapper.adoptOpen(context.Background(), context.Background(), OwnerBinding{SessionID: "native-session"})
	if !errors.Is(err, errNativeOwnerClosed) {
		t.Fatalf("closing-only Open adoption = %v", err)
	}
	if wrapper.opened {
		t.Fatal("closing-only Open was committed")
	}
}

func TestOMPAdoptOpenRejectsChangedBindingAndRecordedOwnerFailure(t *testing.T) {
	options, _ := nativeOwnerFixture(t, "")
	owner, err := StartNativeOwner(nativeOwnerTestContext(t), options)
	if err != nil {
		t.Fatal(err)
	}
	waitNativeOwnerReady(t, owner)
	binding, ok := owner.Primary()
	if !ok {
		t.Fatal("ready owner has no primary binding")
	}
	wrapper := &Wrapper{ctx: context.Background(), owner: owner}
	wrong := binding
	wrong.OwnerToken += "-replacement"
	if err = wrapper.adoptOpen(context.Background(), context.Background(), wrong); err == nil || !strings.Contains(err.Error(), "registry changed") {
		t.Fatalf("replacement binding adoption = %v", err)
	}
	if wrapper.opened {
		t.Fatal("replacement binding was committed")
	}

	want := errors.New("recorded registry failure before owner Done")
	owner.registry.recordError(want)
	if err = wrapper.adoptOpen(context.Background(), context.Background(), binding); !errors.Is(err, want) {
		t.Fatalf("recorded owner failure adoption = %v", err)
	}
	if wrapper.opened {
		t.Fatal("failing owner was committed")
	}
	select {
	case <-owner.Done():
		t.Fatal("test did not preserve the recorded-error-before-Done interval")
	default:
	}
	if err = owner.Close(nativeOwnerTestContext(t)); !errors.Is(err, want) {
		t.Fatalf("joined owner cleanup = %v", err)
	}
}

func TestOMPLossShutdownWorkIsAdmittedBeforeCloseWait(t *testing.T) {
	wrapper := &Wrapper{opened: true}
	wrapper.ctx, wrapper.cancel = context.WithCancelCause(context.Background())
	shutdownEntered, releaseShutdown := make(chan struct{}), make(chan struct{})
	wrapper.shutdown = func() {
		close(shutdownEntered)
		<-releaseShutdown
	}
	loss := errors.New("native owner loss")
	wrapper.lose(loss)
	<-shutdownEntered
	closed := make(chan error, 1)
	go func() { closed <- wrapper.Close(context.Background(), sessionkit.SessionCloseRequest{}) }()
	select {
	case err := <-closed:
		t.Fatalf("Close escaped owned shutdown work: %v", err)
	default:
	}
	close(releaseShutdown)
	if err := <-closed; !errors.Is(err, loss) {
		t.Fatalf("Close = %v", err)
	}

	late := &Wrapper{opened: true}
	late.ctx, late.cancel = context.WithCancelCause(context.Background())
	called := false
	late.shutdown = func() { called = true }
	if err := late.Close(context.Background(), sessionkit.SessionCloseRequest{}); err != nil {
		t.Fatal(err)
	}
	late.lose(errors.New("late loss"))
	if called {
		t.Fatal("loss admitted shutdown work after Close began waiting")
	}
}
