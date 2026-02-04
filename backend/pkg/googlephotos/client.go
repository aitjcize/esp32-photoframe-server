package googlephotos

import (
	"context"
	"net/http"

	"golang.org/x/oauth2"
)

type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type ConfigProvider interface {
	GetGoogleConfig() (Config, error)
}

type TokenStore interface {
	GetToken() (*oauth2.Token, error)
	SaveToken(*oauth2.Token) error
	ClearToken() error
}

type Client struct {
	configProvider ConfigProvider
	store          TokenStore
	client         *http.Client
	redirectURL    string
}

func NewClient(provider ConfigProvider, store TokenStore) *Client {
	return &Client{
		configProvider: provider,
		store:          store,
	}
}

func (c *Client) SetRedirectURL(url string) {
	c.redirectURL = url
}

func (c *Client) getOAuthConfig() (*oauth2.Config, error) {
	cfg, err := c.configProvider.GetGoogleConfig()
	if err != nil {
		return nil, err
	}
	// Use dynamically set redirect URL if available, otherwise fall back to config
	redirectURL := c.redirectURL
	if redirectURL == "" {
		redirectURL = cfg.RedirectURL
	}
	return &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  redirectURL,
		Scopes: []string{
			"https://www.googleapis.com/auth/photospicker.mediaitems.readonly",
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		},
	}, nil
}

func (c *Client) GetAuthURL() string {
	conf, err := c.getOAuthConfig()
	if err != nil {
		return "" // TODO: Handle error better?
	}
	// prompt=consent ensures we get a refresh token
	return conf.AuthCodeURL("state", oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))
}

func (c *Client) Exchange(code string) error {
	conf, err := c.getOAuthConfig()
	if err != nil {
		return err
	}
	token, err := conf.Exchange(context.Background(), code)
	if err != nil {
		return err
	}
	if err := c.store.SaveToken(token); err != nil {
		return err
	}

	// Reset the cached client so the next request rebuilds it with the new token
	c.client = nil
	return nil
}

func (c *Client) GetClient() (*http.Client, error) {
	if c.client != nil {
		return c.client, nil
	}

	token, err := c.store.GetToken()
	if err != nil {
		return nil, err
	}

	conf, err := c.getOAuthConfig()
	if err != nil {
		return nil, err
	}

	// Use a TokenSource that saves the token whenever it's refreshed
	source := conf.TokenSource(context.Background(), token)
	saveSource := &SavingTokenSource{
		source: source,
		store:  c.store,
	}
	c.client = oauth2.NewClient(context.Background(), oauth2.ReuseTokenSource(token, saveSource))
	return c.client, nil
}

// SavingTokenSource is a wrapper around oauth2.TokenSource that saves refreshed tokens
type SavingTokenSource struct {
	source oauth2.TokenSource
	store  TokenStore
}

func (s *SavingTokenSource) Token() (*oauth2.Token, error) {
	token, err := s.source.Token()
	if err != nil {
		return nil, err
	}
	// Always save the token back to the store. The store implementation
	// should handle deduping if it's the same token.
	if err := s.store.SaveToken(token); err != nil {
		return nil, err
	}
	return token, nil
}

func (c *Client) Logout() error {
	c.client = nil
	return c.store.ClearToken()
}

func (c *Client) IsConnected() bool {
	token, err := c.store.GetToken()
	if err != nil || token == nil {
		return false
	}
	// Connected if it's still valid, or if we have a refresh token to fix it
	return token.Valid() || token.RefreshToken != ""
}
