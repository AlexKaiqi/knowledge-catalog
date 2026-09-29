package identity

// TaihuTokenResult is the OAuth token exchange response DTO shared by the
// login flow and the identity token endpoint. (CAP-ORG-01 B6.)

type TaihuTokenResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}
