package gitea

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"

	"kc/kernel"
)

// ManagedUserRequest contains server-owned provisioning and verified identity
// evidence. AllocationID must be durable before the first external request.
// TrustedBackendID may be supplied only when authentication was against this
// exact Gitea origin; a caller's claimed username alone is insufficient.
type ManagedUserRequest struct {
	Origin           string
	Username         string
	EmailDomain      string
	AuthSourceID     int64
	AllocationID     string
	BackendID        int64
	TrustedBackendID int64
}

type managedUserInfo struct {
	ID       int64  `json:"id"`
	Login    string `json:"login"`
	FullName string `json:"full_name"`
}

// EnsureManagedUser never edits, renames, or resets an existing account. It
// recognizes only a matching immutable backend identity or the first create's
// allocation marker, including when the successful response was lost.
func EnsureManagedUser(input ManagedUserRequest, token string) (int64, error) {
	if input.Username == "" || input.AllocationID == "" || input.BackendID < 0 || input.TrustedBackendID < 0 {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "managed user requires durable ownership evidence")
	}
	ep, err := ParseDSN(strings.TrimRight(input.Origin, "/") + "/" + url.PathEscape(input.Username) + "/probe")
	if err != nil {
		return 0, err
	}
	cli := newClient(ep.API, token)
	cli.http.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	userPath := "/users/" + url.PathEscape(input.Username)
	marker := "kc-managed-user:" + input.AllocationID
	verify := func(info managedUserInfo) (int64, error) {
		if info.ID <= 0 || info.Login != input.Username {
			return 0, kernel.Fail(kernel.ErrPreconditionFailed, "Gitea user identity does not match its reservation")
		}
		if input.BackendID != 0 {
			if info.ID != input.BackendID {
				return 0, kernel.Fail(kernel.ErrPreconditionFailed, "managed Gitea user has been replaced")
			}
			return info.ID, nil
		}
		if info.ID == input.TrustedBackendID || info.FullName == marker {
			return info.ID, nil
		}
		return 0, kernel.Fail(kernel.ErrPreconditionFailed, "Gitea username is already owned by an unrelated account")
	}
	var info managedUserInfo
	status, _, err := cli.do(http.MethodGet, userPath, nil, &info)
	if status != http.StatusNotFound {
		if err != nil {
			return 0, err
		}
		return verify(info)
	}
	if input.BackendID != 0 || input.TrustedBackendID != 0 {
		return 0, kernel.Fail(kernel.ErrPreconditionFailed, "verified Gitea user is missing; restore its original account")
	}
	domain := input.EmailDomain
	if domain == "" {
		domain = "users.kc.invalid"
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return 0, err
	}
	body := struct {
		Username           string `json:"username"`
		Email              string `json:"email"`
		FullName           string `json:"full_name"`
		Password           string `json:"password"`
		SourceID           int64  `json:"source_id,omitempty"`
		LoginName          string `json:"login_name,omitempty"`
		MustChangePassword bool   `json:"must_change_password"`
		SendNotify         bool   `json:"send_notify"`
	}{Username: input.Username, Email: input.Username + "@" + domain, FullName: marker, Password: base64.RawURLEncoding.EncodeToString(secret[:]), SourceID: input.AuthSourceID}
	if input.AuthSourceID != 0 {
		body.LoginName = input.Username
	}
	if _, _, err := cli.do(http.MethodPost, "/admin/users", body, &info); err != nil {
		// A conflict or lost response cannot authorize adopting another user.
		if _, _, retryErr := cli.do(http.MethodGet, userPath, nil, &info); retryErr != nil {
			return 0, err
		}
	}
	return verify(info)
}
