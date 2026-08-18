# Single sign-on through Cloudflare Access

The console can take the identity Cloudflare Access has already
authenticated and turn it into a console session, creating the account on
first sign-in. Someone who reaches the dashboard through your tunnel is
simply *in* — no second login — and you never hand out a console password.

It is optional. Unset the two variables and the console knows only its
password form, exactly as before.

## How it works

Access sits in front of the tunnel. When a request clears its policy,
Cloudflare adds a signed token: the `Cf-Access-Jwt-Assertion` header (and
a `CF_Authorization` cookie on browser navigations). The token names the
person — `email` — and is signed with RS256 by a key your team publishes.

1. The dashboard loads and asks `/api/me`. No session yet, so it gets 401.
2. It quietly posts to `/api/login/cloudflare`. The assertion is already
   on that request; there is nothing for the page to send.
3. `core/cfaccess` verifies the token: signature against the team's
   published keys, issuer, **audience**, expiry, and that it is an
   application token rather than the team-wide session one.
4. The email becomes a console account — found, or created on the
   spot — and the console issues its own session cookie.
5. If any of that fails, the password form appears as usual. Nobody who
   isn't behind Access notices the attempt happened.

## Setting it up

**1. Put the console behind an Access application.** In Zero Trust →
Access controls → Applications, add a self-hosted application for the
hostname your tunnel serves the console on, and give it a policy (an
email list, a group, whatever you already use).

**2. Copy the Application Audience (AUD) tag.** Application → Configure →
Additional settings → *Application Audience (AUD) Tag*. It is a 64-char
hex string and it never changes unless you delete and recreate the
application.

**3. Configure the console:**

```yaml
CF_ACCESS_TEAM_DOMAIN: yourteam.cloudflareaccess.com   # or just "yourteam"
CF_ACCESS_AUD: "<the 64-char AUD tag>"
CF_ACCESS_ADMIN_EMAILS: "you@example.com"              # optional, see below
```

Restart. The log line `cloudflare access sign-in enabled` confirms it.

**4. Recommended — have `cloudflared` check the token too.** The tunnel
can validate the same assertion before it ever reaches the console, so a
request that somehow skipped Access is dropped a hop earlier:

```yml
originRequest:
  access:
    required: true
    teamName: yourteam
    audTag:
      - <the same AUD tag>
```

This is defence in depth, not a replacement: it protects the tunnel path
only, so the console keeps verifying for itself.

## Accounts

**First sign-in creates the account**, named by the email address, with
**no permissions**. That is deliberate. Access answers "who is this?"; it
has no opinion on who may stop your server or read your role passwords,
and a console that granted power to everyone in your identity provider
would be a nasty surprise. Grant permissions in Users afterwards, as you
would for any account.

**`CF_ACCESS_ADMIN_EMAILS` exists to stop you locking yourself out.**
Addresses on that list hold the admin role, re-applied at every sign-in.
Without it the first person through Access lands in a console where
nobody has the rights to promote them. Because it is re-applied, it also
wins over a demotion made in the UI — if you want someone demoted, take
them off the list.

**Disabling an account still works, and outranks Access.** A disabled
user is refused at sign-in whatever your identity provider says, so
revoking someone doesn't have to mean editing the Access policy. They see
the reason on the login page rather than a blank password prompt.

**Accounts created this way have no password.** The stored hash is a
sentinel that bcrypt refuses to parse, so every password comparison
against it fails. There is no password to leak or guess; if such a user
should also be able to log in without Access, set them one in Users.

**Service tokens are refused.** A machine that clears the Access policy
arrives with a `common_name` and no email. The console has no machine
accounts, and inventing one would be a back door with good manners.

## What the verification does and does not protect

**A header is only a header.** Anything that can reach the console
directly — another container on the host, someone on your LAN, a port
forward you forgot — can send `Cf-Access-Jwt-Assertion: whatever`. That
is why the token is verified cryptographically on every use rather than
trusted for existing. A forged header cannot produce a valid RS256
signature.

Two checks beyond the signature carry real weight:

- **Issuer**, so a token from another team is not accepted.
- **Audience**, so a token minted for a *different application in your own
  team* is not accepted. This is the subtle one: every application in your
  organization is signed by the same account-level key and carries the
  same issuer, so a token from your most permissive app is
  cryptographically perfect here. The AUD tag is the only thing that
  separates "authenticated by my team" from "authorized for this console".
  This is why `CF_ACCESS_AUD` is required rather than optional.

**What it does not protect:** the password form. Someone who reaches the
console directly can still try to log in with a username and password.
That is a deliberate trade — it is the break-glass path for when
Cloudflare is unreachable, when you are on the LAN, or when the tunnel is
down — and it is the reason password login was not removed. If that
trade is wrong for you, the fix is at the network edge: don't publish the
container's port, and reach the console only through the tunnel.

**Expiry is checked strictly.** A five-minute skew tolerance applies to
tokens that are not valid *yet* (Access sets `nbf` equal to `iat`, so a
fast origin clock would otherwise reject brand-new tokens), and
deliberately does not extend the life of one that has expired.

## Signing out

Signing out clears the console's session **and** sends the browser to
Access's logout URL. Clearing only ours would be no sign-out at all:
Access would hand the next person at that browser the same identity.

Two Cloudflare behaviours worth knowing, neither ours to change: logging
out ends the session for **every** Access application, there being no
per-application logout; and already-issued tokens keep verifying for
another 20–30 seconds.

## Group-based permissions, and why they aren't wired up

Access can put identity-provider groups in the token, but only if you
configure them as a custom claim — and Cloudflare trims custom claims
when they exceed roughly 1 KB, dropping values from the end. A user in
many groups can silently arrive without their groups. Cloudflare's own
guidance is not to make authorization decisions from that claim; the
authoritative source is a separate `get-identity` call.

So the console maps permissions in its own Users page, and uses the
admin-email list only for lockout rescue. If group-driven roles ever
become worth it, `get-identity` cached on the token's `identity_nonce` is
the route — not the JWT claim.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| Password form appears through the tunnel | `CF_ACCESS_AUD` doesn't match the application's tag, or the variables aren't set — check the startup log for `cloudflare access sign-in enabled` |
| `assertion rejected` in the logs | Wrong AUD (a token from another app in the team), wrong team domain, or a stale token |
| Signed in but nothing is clickable | Working as designed: new accounts hold no permissions. Grant them in Users, or add the address to `CF_ACCESS_ADMIN_EMAILS` |
| "account disabled" on the login page | Access authenticated them; the console has that account disabled |
| Works on the tunnel, not on the LAN | Also working as designed — there is no assertion on a direct request. Use the password form |

## Reference

- Validating tokens, JWKS endpoint, claims:
  developers.cloudflare.com → Cloudflare One → Access controls →
  Applications → HTTP apps → Authorization cookie → *Validating JSON web
  tokens* (older `/cloudflare-one/identity/...` links redirect here).
- Implementation: `core/cfaccess` (verification, with tests covering
  cross-application tokens, algorithm confusion, key rotation and expiry)
  and `core/api/auth.go` (`handleCloudflareLogin`, account handling).
