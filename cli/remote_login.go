package cli

import (
	"fmt"
	clientsession "kc/client/session"
	inv "kc/invocation"
	"kc/invoke"
	"kc/kernel"
	"os"
	"os/exec"
	"runtime"
)

func verbLogin(cx *inv.Invocation) (any, error) {
	interaction := clientsession.LoginInteraction{
		BeforeBrowser: func() error {
			for _, name := range []string{"oauth2-base", "client-id", "resource", "app-name"} {
				if invoke.FlagString(cx.Flags, name) != "" {
					return kernel.Fail(kernel.ErrUsageInvalid, "--%s is deployment-owned; browser login uses Server discovery", name)
				}
			}
			return nil
		},
		OpenAuthorization: func(url string) {
			fmt.Fprintf(os.Stderr, "\n  Opening browser for Taihu authentication...\n")
			fmt.Fprintf(os.Stderr, "  If the browser does not open, visit:\n  %s\n\n", url)
			openBrowser(url)
		},
		Progress: func(phase clientsession.LoginPhase) {
			switch phase {
			case clientsession.WaitingAuthorization:
				fmt.Fprint(os.Stderr, "  Waiting for authorization... (timeout: 5 min)\n")
			case clientsession.AuthorizationPending:
				fmt.Fprint(os.Stderr, ".")
			case clientsession.AuthorizationCompleted:
				fmt.Fprint(os.Stderr, "\n  Authorization completed!\n")
			}
		},
	}
	return clientsession.Login(cx.Context, clientsession.LoginOptions{Server: invoke.FlagString(cx.Flags, "server"), Mode: invoke.FlagString(cx.Flags, "mode"), Principal: invoke.FlagString(cx.Flags, "as"), Token: invoke.FlagString(cx.Flags, "token"), Wait: invoke.FlagBool(cx.Flags, "wait")}, interaction)
}
func verbLogout(cx *inv.Invocation) (any, error) {
	return clientsession.Logout(cx.Context, invoke.FlagString(cx.Flags, "server"))
}
func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "darwin":
		err = exec.Command("open", url).Start()
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Please open this URL manually: %s\n", url)
	}
}
