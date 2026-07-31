package dockerctl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// This file exists for the palagent provisioner, not for palcon: palcon's
// proxy deliberately can't create containers (see the package comment).
// The provisioner is the one component allowed to hold create rights, and
// even it only ever instantiates the locked Palworld template
// (internal/palagent/provisioner.go).

// pullTimeout bounds an image pull; the palagent image is a few hundred
// MB and cached after the first provision.
const pullTimeout = 10 * time.Minute

// ImagePull pulls ref (e.g. ghcr.io/safwyls/palagent:beta), consuming the
// progress stream until the daemon reports completion.
func (c *Client) ImagePull(ctx context.Context, ref string) error {
	name, tag := ref, "latest"
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		name, tag = ref[:i], ref[i+1:]
	}
	ctx, cancel := context.WithTimeout(ctx, pullTimeout)
	defer cancel()
	path := "/images/create?fromImage=" + url.QueryEscape(name) + "&tag=" + url.QueryEscape(tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("docker endpoint unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return dockerError("image pull", resp.StatusCode, body)
	}
	// The stream is JSON progress lines; an error mid-pull arrives as an
	// {"error": ...} line with a 200 status, so scan rather than discard.
	dec := json.NewDecoder(resp.Body)
	for {
		var line struct {
			Error string `json:"error"`
		}
		if err := dec.Decode(&line); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("reading pull stream: %w", err)
		}
		if line.Error != "" {
			return fmt.Errorf("pulling %s: %s", ref, line.Error)
		}
	}
}

// ContainerSpec is the subset of docker's create payload the provisioner
// template needs — nothing privileged is expressible here by design.
type ContainerSpec struct {
	Name  string
	Image string
	// User is uid:gid; empty keeps the image default.
	User string
	Env  []string
	// Binds are host:container volume pairs.
	Binds []string
	// Ports maps host port -> container "port/proto" (e.g. "8211/udp").
	Ports map[int]string
	// RestartUnlessStopped applies docker's unless-stopped policy.
	RestartUnlessStopped bool
}

// ContainerCreate creates (but does not start) a container; pair with the
// existing Start.
func (c *Client) ContainerCreate(ctx context.Context, spec ContainerSpec) (string, error) {
	exposed := map[string]struct{}{}
	bindings := map[string][]map[string]string{}
	for host, cont := range spec.Ports {
		if !strings.Contains(cont, "/") {
			cont += "/tcp"
		}
		exposed[cont] = struct{}{}
		bindings[cont] = append(bindings[cont], map[string]string{"HostPort": strconv.Itoa(host)})
	}
	hostConfig := map[string]any{
		"Binds":        spec.Binds,
		"PortBindings": bindings,
	}
	if spec.RestartUnlessStopped {
		hostConfig["RestartPolicy"] = map[string]any{"Name": "unless-stopped"}
	}
	payload := map[string]any{
		"Image":        spec.Image,
		"Env":          spec.Env,
		"ExposedPorts": exposed,
		"HostConfig":   hostConfig,
	}
	if spec.User != "" {
		payload["User"] = spec.User
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/containers/create?name="+url.QueryEscape(spec.Name), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("docker endpoint unreachable: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusCreated {
		return "", dockerError("container create", resp.StatusCode, respBody)
	}
	var created struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(respBody, &created); err != nil {
		return "", fmt.Errorf("parsing create response: %w", err)
	}
	return created.ID, nil
}
