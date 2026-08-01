package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/maxbeizer/gh-msft/internal/calendar"
	"github.com/maxbeizer/gh-msft/internal/mail"
	"github.com/maxbeizer/gh-msft/internal/mstime"
)

type fakeMail struct {
	inbox      []mail.Message
	listErr    error
	message    mail.Message
	getErr     error
	getIDs     []string
	archived   []string
	archiveErr error
	body       string
	bodyErr    error
	bodyIDs    []string
}

func (f *fakeMail) ListInbox(ctx context.Context, top int, all bool) ([]mail.Message, error) {
	return f.inbox, f.listErr
}

func (f *fakeMail) GetMessage(ctx context.Context, id string) (mail.Message, error) {
	f.getIDs = append(f.getIDs, id)
	if f.getErr != nil {
		return mail.Message{}, f.getErr
	}
	return f.message, nil
}

func (f *fakeMail) Archive(ctx context.Context, id string) error {
	if f.archiveErr != nil {
		return f.archiveErr
	}
	f.archived = append(f.archived, id)
	return nil
}

func (f *fakeMail) Body(ctx context.Context, id string) (string, error) {
	f.bodyIDs = append(f.bodyIDs, id)
	if f.bodyErr != nil {
		return "", f.bodyErr
	}
	return f.body, nil
}

type fakeCal struct {
	events []calendar.Event
	err    error
}

func (f *fakeCal) Upcoming(ctx context.Context, top int) ([]calendar.Event, error) {
	return f.events, f.err
}

type fakeEULA struct {
	called bool
	err    error
}

func (f *fakeEULA) AcceptEULA(ctx context.Context) error {
	f.called = true
	return f.err
}

func factoryFor(m mail.Provider, c calendar.Provider) Factory {
	return func(ctx context.Context) (*Providers, error) {
		return &Providers{Mail: m, Cal: c, Close: func() {}}, nil
	}
}

func factoryWithEULA(e EULAAccepter) Factory {
	return func(ctx context.Context) (*Providers, error) {
		return &Providers{EULA: e, Close: func() {}}, nil
	}
}

func run(t *testing.T, factory Factory, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd(factory)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return buf.String(), err
}

func TestRootCmdNoArgsLaunchesTUI(t *testing.T) {
	var gotTop int
	var gotAll, gotStartCal bool
	runTUI := func(_ mail.Provider, _ calendar.Provider, top int, all bool, startCal bool) error {
		gotTop = top
		gotAll = all
		gotStartCal = startCal
		return nil
	}
	root := newRootCmd(factoryFor(&fakeMail{}, &fakeCal{}), runTUI)
	root.SetArgs(nil)

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run root command: %v", err)
	}
	if gotTop != 50 || gotAll || gotStartCal {
		t.Errorf("TUI options = top:%d all:%t startCal:%t, want top:50 all:false startCal:false", gotTop, gotAll, gotStartCal)
	}
}

func TestRootCmdExplicitSubcommandsDoNotLaunchTUI(t *testing.T) {
	tuiCalled := false
	runTUI := func(mail.Provider, calendar.Provider, int, bool, bool) error {
		tuiCalled = true
		return nil
	}
	root := newRootCmd(factoryFor(&fakeMail{}, &fakeCal{}), runTUI)
	root.SetArgs([]string{"mail", "list"})

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("run mail list: %v", err)
	}
	if tuiCalled {
		t.Error("TUI launched for explicit mail subcommand")
	}
}

func TestRootCmdHelpAndCompletionDoNotLaunchTUI(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"help"}, {"completion", "bash"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			tuiCalled := false
			factoryCalled := false
			runTUI := func(mail.Provider, calendar.Provider, int, bool, bool) error {
				tuiCalled = true
				return nil
			}
			factory := func(context.Context) (*Providers, error) {
				factoryCalled = true
				return nil, errors.New("factory should not be called")
			}
			root := newRootCmd(factory, runTUI)
			root.SetArgs(args)

			if err := root.ExecuteContext(context.Background()); err != nil {
				t.Fatalf("run %q: %v", args, err)
			}
			if tuiCalled || factoryCalled {
				t.Errorf("%q started TUI=%t factory=%t, want both false", args, tuiCalled, factoryCalled)
			}
		})
	}
}

func TestRootCmdRejectsPositionalArguments(t *testing.T) {
	root := newRootCmd(factoryFor(&fakeMail{}, &fakeCal{}), func(mail.Provider, calendar.Provider, int, bool, bool) error {
		t.Fatal("TUI launched with positional arguments")
		return nil
	})
	root.SetArgs([]string{"unexpected"})

	if err := root.ExecuteContext(context.Background()); err == nil {
		t.Fatal("expected positional arguments to be rejected")
	}
}

func TestMailListTable(t *testing.T) {
	fm := &fakeMail{inbox: []mail.Message{
		{ID: "1", Subject: "Hello world", From: mail.Address{Name: "Alice"}, IsRead: false, Received: mstime.Parse("2026-07-17T21:36:12Z")},
	}}
	out, err := run(t, factoryFor(fm, &fakeCal{}), "mail", "list")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "Hello world") || !strings.Contains(out, "Alice") {
		t.Errorf("output missing content:\n%s", out)
	}
	if !strings.Contains(out, "SUBJECT") {
		t.Errorf("output missing header:\n%s", out)
	}
}

func TestMailListJSON(t *testing.T) {
	fm := &fakeMail{inbox: []mail.Message{
		{ID: "1", Subject: "Hello", From: mail.Address{Name: "Alice", Email: "a@x.com"}, IsRead: true, Received: mstime.Parse("2026-07-17T21:36:12Z")},
	}}
	out, err := run(t, factoryFor(fm, &fakeCal{}), "mail", "list", "--json")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var got []mail.Message
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if len(got) != 1 || got[0].Subject != "Hello" {
		t.Errorf("unexpected JSON: %+v", got)
	}
}

func TestMailListEmpty(t *testing.T) {
	out, err := run(t, factoryFor(&fakeMail{}, &fakeCal{}), "mail", "list")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "No messages") {
		t.Errorf("expected empty message, got:\n%s", out)
	}
}

func TestMailView(t *testing.T) {
	message := mail.Message{
		ID:       "MSG1",
		Subject:  "Status update",
		From:     mail.Address{Name: "Ada Lovelace", Email: "ada@example.com"},
		To:       []mail.Address{{Name: "Grace Hopper", Email: "grace@example.com"}},
		Received: mstime.Parse("2026-07-17T21:36:12Z"),
		IsRead:   false,
	}
	tests := []struct {
		name    string
		args    []string
		wantOut []string
		asJSON  bool
	}{
		{
			name:    "human output",
			args:    []string{"mail", "view", "MSG1"},
			wantOut: []string{"Subject: Status update", "From: Ada Lovelace <ada@example.com>", "To: Grace Hopper <grace@example.com>", "plain text body"},
		},
		{
			name:   "json output",
			args:   []string{"mail", "view", "MSG1", "--json"},
			asJSON: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := &fakeMail{message: message, body: "plain text body"}
			out, err := run(t, factoryFor(fm, &fakeCal{}), tt.args...)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if len(fm.getIDs) != 1 || fm.getIDs[0] != "MSG1" || len(fm.bodyIDs) != 1 || fm.bodyIDs[0] != "MSG1" {
				t.Errorf("provider ids = get:%v body:%v", fm.getIDs, fm.bodyIDs)
			}
			if tt.asJSON {
				var got mail.Detail
				if err := json.Unmarshal([]byte(out), &got); err != nil {
					t.Fatalf("invalid JSON output: %v\n%s", err, out)
				}
				if got.ID != message.ID || got.Subject != message.Subject || got.From != message.From || len(got.To) != 1 || got.To[0] != message.To[0] || got.Body != "plain text body" {
					t.Errorf("JSON detail = %+v", got)
				}
				return
			}
			for _, want := range tt.wantOut {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q:\n%s", want, out)
				}
			}
		})
	}
}

func TestMailViewErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		mail *fakeMail
		want string
	}{
		{
			name: "bad arguments",
			args: []string{"mail", "view"},
			mail: &fakeMail{},
			want: "accepts 1 arg(s), received 0",
		},
		{
			name: "metadata provider error",
			args: []string{"mail", "view", "MSG1", "--json"},
			mail: &fakeMail{getErr: errors.New("permission denied")},
			want: "getting message MSG1: permission denied",
		},
		{
			name: "body provider error",
			args: []string{"mail", "view", "MSG1", "--json"},
			mail: &fakeMail{message: mail.Message{ID: "MSG1"}, bodyErr: errors.New("not found")},
			want: "reading body for message MSG1: not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := run(t, factoryFor(tt.mail, &fakeCal{}), tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if out != "" {
				t.Errorf("stdout = %q, want empty on error", out)
			}
		})
	}
}

func TestMailArchiveArgs(t *testing.T) {
	fm := &fakeMail{}
	out, err := run(t, factoryFor(fm, &fakeCal{}), "mail", "archive", "ID1", "ID2")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(fm.archived) != 2 || fm.archived[0] != "ID1" || fm.archived[1] != "ID2" {
		t.Errorf("archived = %v", fm.archived)
	}
	if !strings.Contains(out, "archived ID1") {
		t.Errorf("missing confirmation:\n%s", out)
	}
}

func TestMailArchiveNoIDsErrors(t *testing.T) {
	_, err := run(t, factoryFor(&fakeMail{}, &fakeCal{}), "mail", "archive")
	if err == nil {
		t.Fatal("expected error when no ids provided")
	}
}

func TestMailArchivePropagatesError(t *testing.T) {
	fm := &fakeMail{archiveErr: errors.New("boom")}
	_, err := run(t, factoryFor(fm, &fakeCal{}), "mail", "archive", "X")
	if err == nil {
		t.Fatal("expected archive error to propagate")
	}
}

func TestCalTable(t *testing.T) {
	fc := &fakeCal{events: []calendar.Event{
		{ID: "E1", Subject: "Standup", Organizer: "Max", Start: mstime.Parse("2026-07-20T16:30:00.0000000"), End: mstime.Parse("2026-07-20T17:00:00.0000000")},
	}}
	out, err := run(t, factoryFor(&fakeMail{}, fc), "cal")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "Standup") || !strings.Contains(out, "Max") {
		t.Errorf("output missing content:\n%s", out)
	}
}

func TestCalTableAllDay(t *testing.T) {
	fc := &fakeCal{events: []calendar.Event{
		{ID: "H1", Subject: "Company Holiday", Organizer: "HR", IsAllDay: true,
			Start: mstime.Parse("2026-07-20T00:00:00.0000000"), End: mstime.Parse("2026-07-21T00:00:00.0000000")},
	}}
	out, err := run(t, factoryFor(&fakeMail{}, fc), "cal")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "all day") {
		t.Errorf("all-day event should render \"all day\":\n%s", out)
	}
	if strings.Contains(out, "00:00") {
		t.Errorf("all-day event should not show a time:\n%s", out)
	}
}

func TestCalJSON(t *testing.T) {
	fc := &fakeCal{events: []calendar.Event{
		{ID: "E1", Subject: "Standup", Organizer: "Max", Start: mstime.Parse("2026-07-20T16:30:00.0000000")},
	}}
	out, err := run(t, factoryFor(&fakeMail{}, fc), "cal", "--json")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var got []calendar.Event
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(got) != 1 || got[0].Subject != "Standup" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestAcceptEULACmd(t *testing.T) {
	fe := &fakeEULA{}
	out, err := run(t, factoryWithEULA(fe), "accept-eula")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !fe.called {
		t.Error("AcceptEULA was not called")
	}
	if !strings.Contains(out, "EULA accepted") {
		t.Errorf("missing confirmation:\n%s", out)
	}
}

func TestAcceptEULACmdPropagatesError(t *testing.T) {
	fe := &fakeEULA{err: errors.New("boom")}
	_, err := run(t, factoryWithEULA(fe), "accept-eula")
	if err == nil {
		t.Fatal("expected accept-eula error to propagate")
	}
}

func TestFactoryErrorSurfacesHint(t *testing.T) {
	failing := func(ctx context.Context) (*Providers, error) {
		return nil, errors.New("spawn failed")
	}
	_, err := run(t, failing, "mail", "list")
	if err == nil || !strings.Contains(err.Error(), "WorkIQ") {
		t.Fatalf("expected WorkIQ hint in error, got %v", err)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		in   string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"exactly-ten", 11, "exactly-ten"},
		{"this is a long subject line", 10, "this is a…"},
		{"multi\nline", 20, "multi line"},
	}
	for _, tt := range tests {
		if got := truncate(tt.in, tt.n); got != tt.want {
			t.Errorf("truncate(%q,%d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
	}
}
