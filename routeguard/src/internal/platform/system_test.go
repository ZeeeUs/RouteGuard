package platform_test

import (
	"context"
	"strings"
	"testing"

	"github.com/ZeeeUs/RouteGuard/internal/config"
	"github.com/ZeeeUs/RouteGuard/internal/platform"
	"github.com/ZeeeUs/RouteGuard/internal/platform/mock"
	"go.uber.org/mock/gomock"
)

func TestMatchesConfigChecksAnonymousProfilesInBothFormats(t *testing.T) {
	plain := `package routeguard
config settings 'main'
	option enabled '1'
config service
	option label 'Example'
	list ip '192.0.2.10'
`
	named := strings.Replace(plain, "config service\n", "config service 'cfg025996'\n", 1)

	for _, tc := range []struct {
		name          string
		saved         string
		requiresNamed bool
		want          bool
	}{
		{"positional snapshot", plain, false, true},
		{"LuCI snapshot", named, true, true},
		{"unapplied IP change", strings.Replace(named, "192.0.2.10", "192.0.2.11", 1), true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			applied, err := config.Parse([]byte(tc.saved))
			if err != nil {
				t.Fatal(err)
			}

			runner := mock.NewMockRunner(gomock.NewController(t))
			runner.EXPECT().Run(gomock.Any(), "", "uci", "-q", "-N", "export", "routeguard").Return([]byte(plain), nil)
			if tc.requiresNamed {
				runner.EXPECT().Run(gomock.Any(), "", "uci", "-q", "-n", "export", "routeguard").Return([]byte(named), nil)
			}

			system := platform.System{Commands: runner}
			matches, err := system.MatchesConfig(context.Background(), applied)
			if err != nil || matches != tc.want {
				t.Fatalf("MatchesConfig() = %t, %v; want %t, nil", matches, err, tc.want)
			}
		})
	}
}
