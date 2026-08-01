package browser

import (
	"errors"
	"reflect"
	"testing"
)

func TestCommandForURL(t *testing.T) {
	tests := []struct {
		name     string
		goos     string
		rawURL   string
		wantName string
		wantArgs []string
		wantErr  bool
	}{
		{"macOS", "darwin", "https://teams.microsoft.com/l/meetup-join/abc", "open", []string{"https://teams.microsoft.com/l/meetup-join/abc"}, false},
		{"Linux", "linux", "https://outlook.office.com/calendar/item", "xdg-open", []string{"https://outlook.office.com/calendar/item"}, false},
		{"Windows", "windows", "https://outlook.office.com/calendar/item", "rundll32.exe", []string{"url.dll,FileProtocolHandler", "https://outlook.office.com/calendar/item"}, false},
		{"rejects non-web URL", "darwin", "file:///tmp/event", "", nil, true},
		{"rejects malformed URL", "darwin", "not a URL", "", nil, true},
		{"rejects unsupported platform", "plan9", "https://example.com", "", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotArgs, err := commandForURL(tt.goos, tt.rawURL)
			if (err != nil) != tt.wantErr {
				t.Fatalf("commandForURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if gotName != tt.wantName {
				t.Errorf("commandForURL() name = %q, want %q", gotName, tt.wantName)
			}
			if len(gotArgs) != len(tt.wantArgs) {
				t.Fatalf("commandForURL() args = %v, want %v", gotArgs, tt.wantArgs)
			}
			for i, arg := range gotArgs {
				if arg != tt.wantArgs[i] {
					t.Errorf("commandForURL() arg %d = %q, want %q", i, arg, tt.wantArgs[i])
				}
			}
		})
	}
}

func TestOpenURLPropagatesLauncherResult(t *testing.T) {
	tests := []struct {
		name    string
		runErr  error
		wantErr bool
	}{
		{"successful launcher", nil, false},
		{"failed launcher", errors.New("exit status 1"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotName string
			var gotArgs []string
			err := openURL("linux", "https://outlook.office.com/calendar/item", func(name string, args ...string) error {
				gotName = name
				gotArgs = args
				return tt.runErr
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("openURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if gotName != "xdg-open" || !reflect.DeepEqual(gotArgs, []string{"https://outlook.office.com/calendar/item"}) {
				t.Errorf("launcher = %q %v, want xdg-open [https://outlook.office.com/calendar/item]", gotName, gotArgs)
			}
		})
	}
}
