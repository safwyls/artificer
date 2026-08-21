// Package igdb resolves game artwork and titles from IGDB, so a list of
// installed folder names can be shown as a shelf of covers.
//
// It lives server-side on purpose. The credentials are a Twitch client
// id and secret, and putting them in the companion would mean shipping
// them to every player's machine; here one deployment holds them, every
// companion asks through its own sync token, and the answers are cached
// once for everyone.
//
// Nothing here is load-bearing: artwork is decoration over metadata the
// companion already reports. Every failure — no credentials, a rate
// limit, IGDB down — degrades to "no cover" rather than an error the
// player has to care about.
package igdb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	tokenURL = "https://id.twitch.tv/oauth2/token"
	apiURL   = "https://api.igdb.com/v4"
	// steamCategory is IGDB's external_games category for Steam.
	steamCategory = 1
	// coverSize is IGDB's 264x374 cover; big enough for a grid tile at
	// 2x without pulling full-size art.
	coverSize = "t_cover_big"

	hitTTL  = 7 * 24 * time.Hour
	missTTL = 6 * time.Hour
	// maxNameLookups bounds a single batch's search queries — IGDB's
	// rate limit is four requests a second, and a big Steam library
	// would otherwise fire one per unmatched game.
	maxNameLookups = 12
)

// Game is what a lookup yields: IGDB's title and a cover image URL.
// Both may be empty, which means "IGDB knows nothing about this" and is
// a perfectly ordinary answer.
type Game struct {
	Name     string `json:"name,omitempty"`
	Cover    string `json:"cover,omitempty"`
	Summary  string `json:"summary,omitempty"`
	IGDBSlug string `json:"slug,omitempty"`
}

type entry struct {
	game Game
	at   time.Time
	miss bool
}

type Client struct {
	id, secret string
	http       *http.Client

	mu     sync.Mutex
	token  string
	expiry time.Time
	cache  map[string]entry
}

// New returns a client, or nil when either credential is missing — a
// nil *Client is usable and simply answers nothing, so callers never
// branch on configuration.
func New(clientID, clientSecret string) *Client {
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(clientSecret) == "" {
		return nil
	}
	return &Client{
		id:     strings.TrimSpace(clientID),
		secret: strings.TrimSpace(clientSecret),
		http:   &http.Client{Timeout: 15 * time.Second},
		cache:  map[string]entry{},
	}
}

// Query is one game to look up: the Steam app id when known (exact), and
// the name as a fallback.
type Query struct {
	AppID string `json:"appId,omitempty"`
	Name  string `json:"name,omitempty"`
}

// Key is how a query's answer is addressed in the result map — the app
// id when there is one, else the lowercased name.
func (q Query) Key() string {
	if q.AppID != "" {
		return "app:" + q.AppID
	}
	return "name:" + strings.ToLower(strings.TrimSpace(q.Name))
}

// Lookup resolves a batch, cache first. The map is keyed by Query.Key();
// games IGDB doesn't know are simply absent.
func (c *Client) Lookup(ctx context.Context, queries []Query) map[string]Game {
	out := map[string]Game{}
	if c == nil {
		return out
	}
	var needAppIDs []string
	var needNames []Query

	c.mu.Lock()
	for _, q := range queries {
		key := q.Key()
		if e, ok := c.cache[key]; ok && time.Since(e.at) < ttlFor(e) {
			if !e.miss {
				out[key] = e.game
			}
			continue
		}
		if q.AppID != "" {
			needAppIDs = append(needAppIDs, q.AppID)
		} else if q.Name != "" {
			needNames = append(needNames, q)
		}
	}
	c.mu.Unlock()

	if len(needAppIDs) > 0 {
		found := c.byAppIDs(ctx, needAppIDs)
		for _, id := range needAppIDs {
			key := "app:" + id
			g, ok := found[id]
			c.remember(key, g, !ok)
			if ok {
				out[key] = g
			}
		}
	}
	for i, q := range needNames {
		if i >= maxNameLookups {
			break
		}
		g, ok := c.byName(ctx, q.Name)
		c.remember(q.Key(), g, !ok)
		if ok {
			out[q.Key()] = g
		}
	}
	return out
}

func ttlFor(e entry) time.Duration {
	if e.miss {
		return missTTL
	}
	return hitTTL
}

func (c *Client) remember(key string, g Game, miss bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = entry{game: g, at: time.Now(), miss: miss}
}

// accessToken fetches or reuses the client-credentials token. Twitch
// tokens last ~60 days; the minute of slack keeps a request from racing
// an expiry.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.token != "" && time.Now().Before(c.expiry.Add(-time.Minute)) {
		defer c.mu.Unlock()
		return c.token, nil
	}
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf(
		"%s?client_id=%s&client_secret=%s&grant_type=client_credentials", tokenURL, c.id, c.secret), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("igdb token: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	c.mu.Lock()
	c.token = out.AccessToken
	c.expiry = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	c.mu.Unlock()
	return out.AccessToken, nil
}

// query runs one APIcalypse request against an IGDB endpoint.
func (c *Client) query(ctx context.Context, endpoint, body string, out any) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/"+endpoint, bytes.NewReader([]byte(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Client-ID", c.id)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("igdb %s: %s: %s", endpoint, resp.Status, strings.TrimSpace(string(raw)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func coverURL(imageID string) string {
	if imageID == "" {
		return ""
	}
	return "https://images.igdb.com/igdb/upload/" + coverSize + "/" + imageID + ".jpg"
}

// byAppIDs resolves Steam app ids in one request — the exact path, since
// the id came from the game's own Steam manifest.
func (c *Client) byAppIDs(ctx context.Context, ids []string) map[string]Game {
	quoted := make([]string, 0, len(ids))
	for _, id := range ids {
		quoted = append(quoted, `"`+id+`"`)
	}
	body := fmt.Sprintf(
		`fields uid,game.name,game.slug,game.summary,game.cover.image_id; where category = %d & uid = (%s); limit %d;`,
		steamCategory, strings.Join(quoted, ","), len(ids))
	var rows []struct {
		UID  string `json:"uid"`
		Game struct {
			Name    string `json:"name"`
			Slug    string `json:"slug"`
			Summary string `json:"summary"`
			Cover   struct {
				ImageID string `json:"image_id"`
			} `json:"cover"`
		} `json:"game"`
	}
	if err := c.query(ctx, "external_games", body, &rows); err != nil {
		return nil
	}
	out := map[string]Game{}
	for _, r := range rows {
		if r.Game.Name == "" {
			continue
		}
		out[r.UID] = Game{
			Name:     r.Game.Name,
			Cover:    coverURL(r.Game.Cover.ImageID),
			Summary:  r.Game.Summary,
			IGDBSlug: r.Game.Slug,
		}
	}
	return out
}

// byName is the fallback for a game with no Steam app id — a folder
// name from common/, say.
func (c *Client) byName(ctx context.Context, name string) (Game, bool) {
	safe := strings.ReplaceAll(name, `"`, "")
	body := fmt.Sprintf(`search "%s"; fields name,slug,summary,cover.image_id; limit 1;`, safe)
	var rows []struct {
		Name    string `json:"name"`
		Slug    string `json:"slug"`
		Summary string `json:"summary"`
		Cover   struct {
			ImageID string `json:"image_id"`
		} `json:"cover"`
	}
	if err := c.query(ctx, "games", body, &rows); err != nil || len(rows) == 0 {
		return Game{}, false
	}
	r := rows[0]
	return Game{Name: r.Name, Cover: coverURL(r.Cover.ImageID), Summary: r.Summary, IGDBSlug: r.Slug}, true
}
