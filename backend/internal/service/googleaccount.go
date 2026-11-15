package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/googlephotos"
)

// googleAccount is the OAuth side of one connected Google account: the token
// kept on its connection row, the consent URL, the code exchange and the
// account email. The Google Photos and Google Calendar connection services
// embed it and differ only in the scopes their config provider asks for and
// in what they do with the authorised client.
type googleAccount struct {
	id     uint
	store  *connectionTokenStore
	cfg    googlephotos.ConfigProvider
	client *googlephotos.Client
}

func newGoogleAccount(conns *ConnectionStore, id uint, cfg googlephotos.ConfigProvider) *googleAccount {
	store := &connectionTokenStore{conns: conns, id: id}
	return &googleAccount{
		id:     id,
		store:  store,
		cfg:    cfg,
		client: googlephotos.NewClient(cfg, store),
	}
}

// ConnectionID is the connection this account belongs to.
func (a *googleAccount) ConnectionID() uint { return a.id }

// Connected reports whether the account has a usable token.
func (a *googleAccount) Connected() bool {
	st, err := a.store.state()
	return err == nil && st.Connected()
}

// AuthURL returns the Google consent page for this account. redirectURL is the
// server's callback as the browser reaches it; state ties the callback to this
// connection.
func (a *googleAccount) AuthURL(redirectURL, state string) (string, error) {
	cfg, err := a.cfg.GetGoogleConfig()
	if err != nil {
		return "", err
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return "", errors.New("Google API credentials are not configured; enter the client id and secret first")
	}
	a.client.SetRedirectURL(redirectURL)
	url := a.client.GetAuthURL(state)
	if url == "" {
		return "", errors.New("could not build the Google sign-in URL")
	}
	return url, nil
}

// Exchange turns the callback's code into a token for this account and records
// the account email for display. redirectURL must be the one AuthURL was given.
func (a *googleAccount) Exchange(redirectURL, code string) error {
	a.client.SetRedirectURL(redirectURL)
	if err := a.client.Exchange(code); err != nil {
		return err
	}
	if email, err := a.fetchEmail(); err != nil {
		log.Printf("google connection #%d: account email lookup: %v", a.id, err)
	} else if err := a.store.setEmail(email); err != nil {
		log.Printf("google connection #%d: store email: %v", a.id, err)
	}
	return nil
}

// fetchEmail reads the connected account's address (the userinfo.email scope
// is part of the consent).
func (a *googleAccount) fetchEmail() (string, error) {
	httpClient, err := a.client.GetClient()
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("userinfo: status %d: %s", resp.StatusCode, body)
	}
	var info struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	return info.Email, nil
}

// HTTPClient is an authorised client for Google's APIs, refreshing the token
// in place as needed.
func (a *googleAccount) HTTPClient() (*http.Client, error) {
	return a.client.GetClient()
}

// Logout forgets the token; the connection row stays.
func (a *googleAccount) Logout() error {
	return a.client.Logout()
}

// Email is the account address recorded at sign-in ("" before that).
func (a *googleAccount) Email() string {
	st, err := a.store.state()
	if err != nil {
		return ""
	}
	return st.Email
}
