package requests

import "encoding/base64"

// basicAuth implements HTTP Basic authentication.
type basicAuth struct {
	user, pass string
}

// BasicAuth returns an Authenticator that performs HTTP Basic authentication,
// equivalent to requests.auth.HTTPBasicAuth(username, password).
func BasicAuth(user, pass string) Authenticator {
	return &basicAuth{user: user, pass: pass}
}

// Apply sets the Authorization header for the prepared request.
func (a *basicAuth) Apply(p *PreparedRequest) error {
	token := base64.StdEncoding.EncodeToString([]byte(a.user + ":" + a.pass))
	p.Headers.Set("Authorization", "Basic "+token)
	return nil
}

// bearerAuth implements Bearer token authentication.
type bearerAuth struct {
	token string
}

// BearerAuth returns an Authenticator that sends a Bearer token.
func BearerAuth(token string) Authenticator {
	return &bearerAuth{token: token}
}

// Apply sets the Authorization header for the prepared request.
func (a *bearerAuth) Apply(p *PreparedRequest) error {
	p.Headers.Set("Authorization", "Bearer "+a.token)
	return nil
}
