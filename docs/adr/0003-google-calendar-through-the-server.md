# ADR 0003 — Read Google Calendar through the Momentum server

- **Date:** 2026-10-29
- **Status:** Accepted: decisions D1–D9 were approved on 2026-10-29, each as recommended, and are recorded in the [implementation plan](../plans/google-calendar.md#decisions-2026-10-23-approved-2026-10-29). Not implemented yet.
- **Source:** User conversation on counting Google Calendar meetings in Plan Today
- **Delivery:** [Google Calendar implementation plan](../plans/google-calendar.md)
- **Numbering:** ADR numbers are shared across the repository ([ADR 0002](./0002-go-postgres-backend.md)).

## Context

Plan Today counts the meetings of the day against its capacity. The only source of meetings today is a local `.ics` file (or, on the Mac, the Calendar app). [`tui/DESIGN.md`](../../tui/DESIGN.md) criterion 18 and [`tui/README.md`](../../tui/README.md) say calendar awareness is local and that "Momentum never fetches, edits, or persists event details".

People keep their meetings in Google Calendar. An `.ics` export has to be refreshed by hand, and the file parser does not read `EXDATE` or `RECURRENCE-ID`, so a cancelled or moved occurrence of a repeating meeting still counts at its old time. Google's own API returns single occurrences with those exceptions applied.

Forces:

- The server already has a Google OAuth client for sign-in, and Cloud Run, Neon and Secret Manager are already set up ([`backend/DEPLOY.md`](../../backend/DEPLOY.md)).
- A refresh token is the first secret the server must be able to use later; until now it kept only hashes of its own tokens.
- The terminal app and every Mac should not each need their own Google connection.
- Event titles are private. They reach the apps only through `plan` (Plan Today D3), and the server never logs them.

## Decision

Momentum reads Google Calendar, read-only, through the Momentum server. Plan decisions:

| ID | Decision |
|---|---|
| D1 | The server keeps one Google connection per account and serves the day's events to every app. |
| D2 | The scope is `calendar.events.readonly`: titles, times, all-day, free/busy and attendance. |
| D3 | Only the primary calendar counts. |
| D4 | A declined meeting is not busy; tentative and unanswered ones are. |
| D5 | The refresh token is stored encrypted with AES-256-GCM under `CALENDAR_TOKEN_KEY`. Without the key the server offers no Google Calendar. |
| D6 | Each machine opts in with `[calendar] source = "google"` in `config.toml`, which the Mac's Calendar tab writes. macOS Calendar stays a Mac-only preference. |
| D7 | `load` reuses Google busy time for up to five minutes; opening Plan Today always fetches. |
| D8 | The Google client is published in production, unverified, so refresh tokens do not expire after seven days. |
| D9 | A macOS Calendar choice on the Mac counts over `config.toml`; choosing Google or Calendar files in the Mac's picker clears it and writes `source`. |

Settled by the code and the existing rules, not chosen here: events are read with `singleEvents=true` and without deleted events, between the start and end of the device's day; working hours, `include_all_day` and `include_transparent` apply to Google events as to files; a Google failure makes the calendar unavailable and planning goes on with tasks (Plan Today D6); the server never stores or logs events or titles; connecting is its own browser flow, separate from sign-in, with a state cookie and PKCE, and its state works once.

This supersedes the "never fetches" wording of criterion 18 and of the TUI README, which G12 rewrites. Reading local files stays as it is.

## Options considered

| Option | Why not |
|---|---|
| Each app connects to Google on its own (loopback OAuth, tokens in the Keychain and `credentials.json`) | Keeps tokens off the server, but needs two implementations and a connection per device. |
| Fetch Google's "secret iCal address" | The link is a bearer secret anyone can use, and the `.ics` parser cannot read repeating-event exceptions. |
| macOS Calendar as the way to Google | Stays available through Settings, but serves only the Mac. |
| A free/busy-only scope | Reveals less, but loses titles on hover and the declined status. Both scopes are sensitive, so the consent screen warns either way. |
| Store the refresh token in plaintext behind Neon's access control | A database leak alone would expose calendars. |
| Leave the client in Testing | Google expires refresh tokens after seven days, so the connection would need redoing weekly. |

## Consequences

Easier:

- One connection serves the terminal app and every Mac; a new machine only turns Google on.
- Repeating meetings with cancelled or moved occurrences count correctly, which files cannot do.
- Declined meetings stop taking time off the day.

Harder, or new work:

- The server holds a credential that reads calendars. A database leak without `CALENDAR_TOKEN_KEY` does not expose it; losing or rotating the key means reconnecting.
- Four Google Cloud steps that no agent does: enable the Calendar API, add the scope to the consent screen, add the redirect URIs, and create the `momentum-calendar-token-key` secret.
- Connecting shows Google's "unverified app" warning once per connect (D8) until the app is verified.
- The two sources can differ: files cannot say a meeting was declined.
- The sidebar can lag a new meeting by up to five minutes (D7); the Plan Today sheet never does.

To revisit:

- Choosing calendars other than the primary one needs `calendar.calendarlist.readonly` and a picker in each app.
- Verifying the Google client removes the warning but takes Google's review.
- Writing to Google Calendar, or showing events anywhere but Plan Today, is out of scope and needs a new ADR.
