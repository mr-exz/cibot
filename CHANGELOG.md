# Changelog

## [0.0.81]

<!-- Prepare for next release: remove this line and write your release notes -->


## [0.0.80]

### Changed
- `/thread` confirmation message now shows clear numbered steps explaining the workflow: ①Join the tech group → ②Go to topic → ③Use /close when done; numbered action buttons guide users through each step (e.g. "1 Join group", "2 Go to topic")
- Daily on-duty reminders now include working hours and timezone for each person, converted to the group's configured timezone (same format as `/oncall` command); reminders are silent for persons outside their working hours or on non-working days — `IsPersonOnline()` check filters them out before scheduling


## [0.0.79]

### Changed
- Status system simplified to three states: **online** (no status), **busy** 🔴 (temporarily unavailable — still assigned in rotation, tasks stay on them), **absent** 🏖 (out for the day — fully removed from rotation, next person takes over)
- Removed lunch, brb, and away statuses — replaced by a single "busy" state; existing DB records for old statuses display as "Busy" automatically
- `/status` keyboard now shows two buttons: Busy and Absent, plus Back
- `/status` now shows the full Mon–Sun rotation schedule for every category the person belongs to, with the current day marked and their own duty days highlighted as "(you)"


## [0.0.78]

### Added
- Daily on-duty reminders: at the start of each on-duty person's shift (their configured `work_hours` start in their timezone), the bot posts a reminder in the support topic (or group if no topic) listing who is on duty and for which category
- `/reminder_on` (admin DM) — enables reminders globally and prints the schedule for today and tomorrow: time, person, category, and destination group/topic
- `/reminder_off` (admin DM) — disables all reminders
- Reminders survive bot restarts — scheduler recomputes the day's plan on startup; categories with no assigned persons are skipped automatically
- Reminder scheduler tracks active timers and cancels them before rescheduling — prevents double-pinging if `/reminder_on` is called more than once in the same day

### Changed
- Linear username is no longer shown in ticket/support issue descriptions and the "not linked to Linear" warning is no longer shown during `/ticket` and `/ticket_manual` flows; `/mylinear` command still works and stores the username for potential future use


## [0.0.77]

### Added
- `/thread`: when the replied-to message contains media, the file is now downloaded from Telegram, uploaded to Linear, and embedded in the issue description — same behaviour as `/ticket` (added in 0.0.76)


## [0.0.76]

### Added
- `/ticket`: when the replied-to message contains media (photo, video, audio, voice, document, animation, video note), the file is now downloaded from Telegram, uploaded to Linear, and embedded directly in the issue description as `![name](url)` for images or `[name](url)` for other files — same inline-preview approach as tech thread close; files over 25 MB are skipped


## [0.0.75]

### Changed
- Tech thread close: media files are now embedded directly in the Linear comment using `![name](url)` for images (renders inline preview) and `[name](url)` for other files, instead of creating sidebar `attachmentCreate` links; all uploads happen before the comment is posted so everything appears in one comment


## [0.0.74]

### Fixed
- Linear file upload: reverted parameter names back to `contentType` and `size` (confirmed correct via schema introspection); the 0.0.73 change to `mimeType`/`filesize` was based on incorrect docs and caused "Unknown argument" errors


## [0.0.73]

### Fixed
- Linear file upload: `fileUpload` mutation parameters are `mimeType` and `filesize`, not `contentType` and `size` — wrong names were silently accepted by GraphQL but generated an invalid pre-signed URL, causing S3 to reject the PUT with 400; also set explicit `Content-Type` and `Content-Length` on the PUT request


## [0.0.72]

### Fixed
- Linear file upload: `uploadUrl` and `assetUrl` are nested inside `uploadFile { ... }` on the `UploadPayload` type, not at the top level — fixes "must have a selection of subfields" error that caused all media uploads to fail


## [0.0.71]

### Fixed
- Linear file upload: `fileUpload` mutation response field is `uploadFile`, not `uploadUrl` — fixes "Cannot query field uploadUrl on type UploadPayload" error that caused all media uploads to fail


## [0.0.70]

### Added
- Tech threads now store files in a per-thread folder (`INFRA-1738/`) instead of a single `.txt` file
- Media messages (photo, video, audio, voice, document, animation, video note) posted in an open tech thread topic are downloaded in the background and saved to the thread folder; files over 25 MB are skipped
- `/close` now responds immediately and runs upload in a background goroutine — waits up to 60 s for any in-flight media downloads to finish before uploading; the 60 s limit also caps the total upload time
- On close: all messages are posted as a Linear comment and each media file is uploaded to Linear via `fileUpload` + `attachmentCreate`; the topic is closed and the folder deleted after upload
- A "✅ Upload complete. Took Xs. N file(s) attached." message is sent in the topic when the goroutine finishes
- `/ticket` and `/ticket_manual` confirmation messages now show a `[Linear: ENG-123]` inline button instead of a raw URL in the text, consistent with `/thread`


## [0.0.69]

### Changed
- `/start` help output is now split into two sections — "In groups" and "In DM" — driven purely by whether a command has `GroupDesc` and/or `Desc` set; commands with only `GroupDesc` appear in groups only, commands with only `Desc` appear in DM only, commands with both appear in both sections with their respective descriptions
- `/ticket`, `/ticket_manual`, `/thread`, `/close` are now group-only (`Desc` cleared) — they require a group/topic context and no longer appear in the DM section
- `/thread` confirmation message now explicitly instructs users to click "Join tech group" to continue in the dedicated topic
- Removed auto-join reporter feature (`addChatMember`) — the Telegram Bot API does not support adding users to groups directly; the "Join tech group" invite link button remains the way to join


## [0.0.68]

### Changed
- `/thread` now automatically adds the reporter (the user whose message was replied to) to the tech group via `addChatMember` and sends a mention in the new topic so they know to continue the conversation there (bot requires "Add Members" permission in the tech group; errors are logged but do not block thread creation)
- Command descriptions cleaned up: action verbs moved to front, "dump" replaced with "post", removed redundant hints from inline text, `GroupDesc` and `Desc` now reflect their respective contexts (group vs DM), `/categories` description clarified from "scopes" to "categories", `/mylinear` changed to "Link or update"


## [0.0.67]

### Changed
- Commands `/thread` and `/close` moved to be available for all

## [0.0.66]

### Changed
- `/thread` now assigns the on-duty person for the selected category to the Linear issue (same logic as `/ticket`); offline warning shown in confirmation if assignee is outside working hours
- Tech thread topic message shows on-call person by name without `@` ping, with a Ping button — clicking it appends "Pinged X by Y at HH:MM" to the message and sends a separate `@username` mention to trigger the notification

## [0.0.65]

### Changed
- `/thread` now assigns the on-duty person for the selected category to the Linear issue (same logic as `/ticket`); an offline note is appended to the issue description if the assignee is outside working hours
- `/thread` confirmation text updated to "✅ Thread opened! Continue the discussion in the topic."

## [0.0.64]

### Changed
- `/thread` confirmation now includes a "Join tech group" button with a permanent invite link so non-members can join directly; link is created once via the Bot API and cached for subsequent threads (bot requires "Invite Users via Link" permission in the tech group)

## [0.0.63]

### Changed
- `/thread` confirmation message now shows two inline URL buttons ("Linear: INFRA-1738" and "Telegram: INFRA-1738") instead of raw URLs
- Tech thread topic name is now just the Linear identifier (e.g. `INFRA-1738`) with no title suffix

## [0.0.62]

### Added
- `/thread` command — reply to any message to create a Linear issue and a dedicated forum topic in the configured tech group (`TECH_GROUP_ID`); the topic is named after the Linear identifier (e.g. `ENG-123: title`); the original message is forwarded into the topic automatically
- `/close` command — run inside a tech thread topic to dump all logged messages as a single Linear comment, close the Telegram topic, and mark the thread closed; the per-thread file is deleted after a successful dump
- Per-thread message log files — every message posted in an open tech thread topic is appended to a plain-text file named after the Linear identifier (e.g. `ENG-123.txt`) stored alongside `messages.csv`
- Tech threads survive bot restarts — open threads are loaded from DB on startup so message tracking resumes automatically
- New env var: `TECH_GROUP_ID`; Linear team key is taken from the selected category (same as `/ticket`)

### Changed
- `CreateIssue` (Linear client) now returns `id`, `identifier`, and `url` instead of just `url`; `identifier` (e.g. `ENG-123`) is used for topic naming and file naming

## [0.0.61]

### Changed
- `/ticket` now requires a reply — running it without replying to a message returns an error with a pointer to `/ticket_manual`
- `/ticket_manual` is a new command for the interactive (guided) flow: describe the issue yourself, then pick category, type, and priority

## [0.0.60]

### Fixed
- `/ticket` standalone (non-reply) flow now correctly asks for a description before showing buttons in all group types — previous fixes in 0.0.57–0.0.59 introduced and resolved forum/topic-group specific regressions; the flow is now stable: description prompt → category/type/priority buttons → issue created with auto-generated title

## [0.0.59]

### Fixed
- `/ticket` (and `/support`) standalone flow in forum/topic groups no longer silently ignores the user's description — the topic-header guard added in 0.0.58 was only applied in `handleTicketStart`, so when it fell back to `handleSupportStart`, `handleSupportStart` saw a non-nil `reply_to_message` and called `handleTicketStart` again, creating infinite mutual recursion; the session was never stored, so the bot received the user's text but had no state to match it against; fixed by applying the same topic-header check in `handleSupportStart` before it redirects to `handleTicketStart`

## [0.0.58]

### Fixed
- `/ticket` standalone (non-reply) in forum/topic groups now correctly asks for a description instead of immediately showing category buttons — Telegram implicitly sets `reply_to_message` to the topic header on every message in a topic, so the bot was treating every standalone `/ticket` in a topic as a reply; fixed by detecting the topic header (its message ID equals `message_thread_id`) and falling back to the standalone flow

## [0.0.57]

### Changed
- `/ticket` standalone (non-reply) flow now asks for a description first, then shows category/type/priority buttons — previously the flow started with buttons and ended with title and description prompts; title is now auto-generated from the first 5 words of the description (matching the reply-based `/ticket` behaviour)

## [0.0.56]

### Changed
- Linear issue description: reporter's Telegram username is now rendered as a clickable link (`[Name](https://t.me/username)`) instead of plain text with `@`; Linear username no longer has a leading `@`

### Fixed
- `/ticket` interactive (non-reply) flow: after priority selection the inline keyboard was not removed when editing the message to the title prompt — Telegram keeps the old keyboard when `reply_markup` is omitted, so the priority buttons (including ❌ Cancel) remained visible; clicking Cancel at this point deleted the session and silently cancelled the flow; fixed by explicitly setting an empty `InlineKeyboardMarkup` when transitioning to the title step

## [0.0.55]

### Added
- Linear issue description now includes the reporter's Linear username (e.g. `John Doe (@tg) / Linear: @john`) when their account is linked via `/mylinear`, making the ticket originator visible in Linear

### Changed
- `/ticket` and standalone `/ticket` no longer block the flow when no Linear account is linked — the category keyboard is shown immediately with a warning: "Your Telegram account is not linked to Linear. Use /mylinear to link it."
- Linear issue labels now include the priority name (`Urgent`, `High`, `Medium`, `Low`) in addition to category and type

### Fixed
- `/ticket` standalone (non-reply) interactive flow now correctly shows the title and description prompts after priority selection — `handleCategoryCallback` and `handleRequestTypeCallback` were missing step guards, allowing Telegram's stale callback replay to reset the session state back to an earlier step and skip the title/description steps
- `/oncall` work hours are now converted from the person's timezone to the group's configured timezone before display (e.g. `08:00-17:00 +03:00` shown as `10:00-19:00 +05:00` in a group set to `+05:00`); group timezone falls back to `UTC` when not configured
- `/rotation` work hours are now converted to the timezone of the group the category belongs to, using a per-chatID cache to avoid redundant DB lookups; global categories fall back to `UTC`
- `/addcategory` categories created for groups with no registered topics were incorrectly saved as global (visible in all groups) instead of being scoped to the selected group — `addCategoryNow` only set `chatID` when `ThreadID != 0`, so the no-topic path always produced `chatID=nil`; fixed by keying off `TargetGroupChatID` instead; success message now correctly says "for this group" rather than "globally" in this case

## [0.0.54]

### Fixed
- Group timezone "Set TZ" flow now correctly accepts offset-format timezones (e.g. `+05:00`) — previously `time.LoadLocation` was used for validation and timezone resolution, which only accepts IANA names and silently rejected all offset strings, so the selection was never saved; replaced with a `parseLocation` helper that tries `time.LoadLocation` first and falls back to `storage.ParseTimezone` for offset strings

## [0.0.53]

### Added
- Group timezone setting — `/groups` now shows the configured timezone next to each group name (defaults to `UTC` when not set) and adds a "🕐 Set TZ" button per group that opens a timezone picker populated from existing person schedules plus common presets (`UTC`, `+03:00`, `+04:00`, `+05:00`, `+06:00`); selected timezone is persisted via migration 009
- `/oncall` Ping time now reflects the group's configured timezone — the ping note shows local time and timezone label (e.g. "Pinged johndoe by alice at 16:35 UTC+5"); falls back to UTC when no group timezone is set

## [0.0.52]

### Changed
- `/oncall` Ping button is now single-use: clicking it edits the original message to remove the keyboard and append "Pinged [user] by [clicker] at HH:MM", then sends a separate `@username` message to trigger the Telegram mention notification

## [0.0.51]

### Changed
- `/start` help output is now organised by role: "User — Group & DM" lists commands usable in both contexts, "User — DM" lists DM-only user commands, and "Admin" lists all admin commands (visible only to admins) — previously a flat Group/DM split mixed user and admin commands in the same section

## [0.0.50]

### Added
- `/oncall` now shows a Ping button for each on-duty person; pressing it sends a new `@username` message into the same chat/topic to trigger a Telegram mention notification
- `/oncall` now shows work hours and timezone for online persons (previously only shown when offline)

### Fixed
- `/oncall` no longer displays the `@` prefix on usernames in the status text, preventing unintended mention notifications on every `/oncall` call
- `/start` Group section no longer shows commands that have no group-specific description (e.g. `/start`, `/version`, `/mylinear`); only commands with an explicit `GroupDesc` (`/ticket`, `/oncall`, `/status`) appear in the Group section

## [0.0.49]

### Changed
- `/status` now shows the person's current availability status and which categories they are on duty for before the change-status buttons, so support persons can see their duty state at a glance from a DM
- `/start` now shows context-aware help: in a group only the public Support commands are listed; in a DM the full command list is shown (admin sections visible to admins only)

## [0.0.48]

### Changed
- `/rotation` now shows full rotation details for every category: group/topic scope, rotation type (daily/weekly), on-duty person with Linear username, work hours, and timezone, and the full team list with each person's current availability indicator; on-duty person is marked in the team list
- `/rotation` team list is now always shown regardless of team size — previously hidden when only one person was assigned

## [0.0.47]

### Added
- `/persons` now contains all person management in one place:
  - **Add person** — "➕ Add person" button at the bottom of the list starts the full add-person flow (category → name → Telegram → Linear → schedule) inline, without a separate command
  - **Add to category** — "➕ Add to category" on a person detail screen shows the category picker and creates a new assignment for that person directly, without re-entering their details
  - **Edit schedule** — "✏️ Edit schedule" on a person detail screen opens the timezone/hours/days pickers pre-filled with the person's current values; Skip preserves the existing value

### Removed
- `/addperson` command — functionality moved into `/persons` (Add person button)
- `/setworkhours` command — functionality moved into `/persons` person detail screen (Edit schedule button)

## [0.0.46]

### Added
- `/persons` admin command — lists all support persons; tap a person to view their schedule, see which categories they are assigned to, remove them from individual categories, or delete them entirely; delete requires a confirmation step

### Fixed
- `/addperson` and `/setworkhours` — schedule picker buttons (timezone, hours, days) now correctly show values entered in previous sessions; previously `INSERT OR IGNORE` silently discarded new timezone/work_hours/work_days when the person already existed in the DB, so those values never reached `GetSupportPersonDefaults` and the picker appeared stuck on old values; changed to `INSERT … ON CONFLICT DO UPDATE` that applies non-empty schedule fields even for existing persons
- `/oncall` — offline indicator now includes the person's timezone next to their work hours (e.g. `offline 🔴 (hours: 09:00-18:00 +05:00)`) so it is clear which timezone the schedule applies to; falls back to `UTC` when no timezone is set

## [0.0.45]

### Changed
- `/addperson` timezone, work hours, and work days steps now show inline picker keyboards populated with values already in use across all support persons in the DB, so admins can reuse existing schedules without typing; manual text entry still works as a fallback; timezone and hours steps have a Skip button (inherits previous value), days step does not
- `/setworkhours` same picker keyboard improvement — after selecting a person, timezone/hours/days steps show the same pickers
- Day picker always includes three preset buttons (1-5, 1-6, 1-7) in addition to any custom values from DB

## [0.0.44]

### Fixed
- `/addperson` — adding a person whose `telegram_username` already exists in another category now reuses the existing record and creates the new assignment instead of failing with a UNIQUE constraint error; same `INSERT OR IGNORE` + SELECT pattern used by `/addtype`

## [0.0.43]

### Fixed
- Group picker buttons (used in `/addcategory`, `/addtopic`, `/setrotation`, `/addtype`, `/addperson`, `/offboard`, `/setlabel`) now appear in stable alphabetical order — previously the order shuffled on every render due to Go map iteration being non-deterministic
- Topic picker buttons (used in `/addcategory` topic selection, `/categories` clone and scope flows) now appear in stable alphabetical order for the same reason
- `/topics` text output now lists groups alphabetically and topics by thread ID within each group

## [0.0.42]

### Changed
- `/topics` now shows `chat_id` next to each group name and `thread_id` as a plain number (e.g. `thread 123 — Name`) to make duplicates and ID mismatches visible
- `/addtopic` group selection step now displays `chat_id` in the header after a group is chosen
- `/addtopic` confirmation message now shows `chat_id`, `thread_id`, and `name` on separate lines instead of a compact summary

## [0.0.41]

### Fixed
- `/status back` now clears the member tag in all approved groups (calls `setChatMemberTag` with empty string) instead of restoring a stale label from the unused `user_labels` table

## [0.0.40]

### Added
- `/oncall` command (public, available to all group members) — shows who is on support duty right now for each category configured in the current topic/group; displays name, `@username`, and availability status (🟢 available / 🔴 offline with work hours / 🍽 lunch / ⏸ BRB / 🚫 away); uses the existing rotation and work-hours logic so the result reflects who would actually handle a ticket right now
- `/status` command (for registered support persons) — opens an inline keyboard with status options: 🍽 Lunch, ⏸ BRB, 🚫 Away, 🟢 Back; tapping a button sets the status in-place; `/status back` clears it and restores the person's member tag; non-support-persons are rejected with a clear message
- `person_status` table (migration 008) — one row per support person, stores current status and time set; `ON DELETE CASCADE` on `support_persons`
- Ticket confirmation shows assignee status if set — if the on-duty person is on lunch/brb/away when a ticket is created, their current status is shown in the confirmation message next to their name
- When status is set, the bot calls `setChatMemberTag` in all approved groups to update the member tag (e.g. "On lunch", "BRB", "Away"); on `/status back`, the stored label (from `/setlabel`) is restored

## [0.0.39]

### Added
- **[Experimental]** `/dns` admin command — manage ps.kz DNS records from Telegram; available to admins only; requires `DNS_EMAIL` and `DNS_PASSWORD` env vars
  - **Accounts** — list all ps.kz billing accounts linked to the configured credentials
  - **List records** — select account, enter domain name, view all DNS records
  - **Add record** — select account, enter domain, name, type (A/AAAA/CNAME/MX/TXT/NS/SRV/CAA), value, TTL; confirm before creating
  - **Delete record** — select account, enter domain, pick record from list; confirm before deleting
  - Uses `github.com/mr-exz/pskz-dns-api` GraphQL client; all operations logged

## [0.0.38]

### Added
- **[Experimental]** Web ticket form at `GET /ticket` — browser-based ticket submission with cascading dropdowns (group → topic → category → request type → priority), reporter name, message body, and optional Telegram message link; creates a Linear issue via the same path as the bot including on-duty assignment
- `/offboard` command — admin flow to remove a departed employee from all bot-managed groups; resolves `@username` to a user ID via the bot's metadata DB, checks membership in each approved group, then shows an inline keyboard to remove from groups one by one or all at once
- `/ticket` reply flow now captures caption text from media messages — previously, replying to a photo+caption message produced an empty ticket body; the caption is now used as the message body
- `/ticket` reply flow now extracts photo and document links from the replied message and includes them in the Linear issue description alongside the message text and source link

### Changed
- Port `8000` exposed in `docker-compose.yml` for the web server; configurable via `WEB_PORT` env var

## [0.0.37]

### Added
- `/users` detail view — tapping a user in the list opens an in-place detail view showing full name, Telegram username, and linked Linear account (or "not linked"), with Set Tag, Delete, and Back actions
- `/users` list shows 🔷 indicator next to users who have a Linear account linked
- `/start` now appears in the help output
- `/ticket` in an unconfigured topic now lists which topics in the same group do have support configured, so users know where to go

### Fixed
- `/users` detail view always showed "not linked" for Linear — `GetUserByID` was not selecting `linear_username`

## [0.0.36]

### Changed
- `/users` list — each user is now a single button; users with a linked Linear account show a 🔷 indicator
- Tapping a user opens a detail view (edited in-place) showing full name, Telegram username, and Linear account (or "not linked"), with Set Tag, Delete, and Back actions
- `/start` added to the command registry so it appears in the help output

## [0.0.35]

### Added
- Linear account linking — on first `/ticket` use, the bot asks the user for their Linear username and saves it against their Telegram user ID; subsequent uses skip the prompt
- `/mylinear` command — lets any user set or update their Linear username at any time; shows current value if already set
- DM access for group members — non-admin users can now DM the bot if they are a member of at least one approved group (checked via `getChatMember`); admin commands remain admin-only
- `/start` command — replaces `/help` as the bot entry point; Telegram shows it automatically as a button on first open
- `linear_username` column added to `telegram_user_metadata` (migration 007)
- `ListApprovedGroupIDs` storage method for membership checks

### Changed
- Text input during an active ticket session is now routed through the pending-session handler for both `FlowTicket` and `FlowSupport`

## [0.0.34]

### Changed
- `/support` and `/ticket` merged into a single `/ticket` command — if used as a reply, the replied-to message is used as the ticket source (immediate creation after category/type/priority); if used standalone, the full support flow runs (category → type → priority → title → description)
- `/support` command removed

## [0.0.33]

### Reverted
- `/support` ForceReply approach from 0.0.32 — reverted back to single-message `EditMessageText` flow; the ForceReply UX was disruptive in group chats

## [0.0.32]

### Fixed
- `/support` title and description steps now use `ForceReply` messages instead of `EditMessageText`; plain text replies from users in groups were never delivered to the bot when group privacy mode is enabled — only replies to bot messages are guaranteed to arrive regardless of privacy mode
- Group-level categories (scoped to a group but not a specific topic) were never returned by `ListCategoriesForContext`; the query now includes `chat_id = ? AND thread_id IS NULL` in both the no-topic and topic branches so group-level categories appear in `/support` for all users in that group

## [0.0.31]

### Added
- Clone category — tap 📋 Clone on any category in `/categories` to duplicate it to a different group/topic; choose target group, then scope (group-level or a specific topic), then keep the same Linear team key or enter a new one; the clone copies the category name, emoji, and all linked request types

### Fixed
- All `**markdown**` syntax removed from Telegram message strings; rule documented in CLAUDE.md — `ParseMode` must never be used in this codebase and markdown syntax renders as literal characters without it
- `msg.From == nil` nil panic on service messages and anonymous admin posts in `handleMessage`
- `/support` title step error now logged when `EditMessageText` fails silently

## [0.0.30]

### Changed
- `/addtype` — after selecting a category, existing request types are now shown as buttons; tap one to link it directly, or tap ✏️ New type to enter a custom name; eliminates duplicate creation and makes reuse of shared types explicit
- Group names are now loaded from DB on cache miss in `getGroupName` so pickers show correct names immediately after bot restart instead of raw IDs
- Group title is only written to DB when it changes (was written on every message); same change-detection pattern as user metadata

## [0.0.29]

### Fixed
- `/addtype` — `AddRequestType` returned a stale connection rowid when the type name already existed (SQLite's `LastInsertId()` after `INSERT OR IGNORE` returns the last rowid of any previous insert on the connection, not zero); always SELECT the id now, so linking an existing type name to a new category works correctly instead of failing with a FOREIGN KEY constraint error

## [0.0.28]

### Added
- Priority selection step in `/support` and `/ticket` flows — after request type (or category if no types), a priority keyboard is shown: 🔴 P0 — now, 🟠 P1 — today, 🟡 P2 — week, 🔵 P3 — later; maps to Linear priority values Urgent/High/Medium/Low and is passed in the `issueCreate` mutation

## [0.0.27]

### Changed
- `/addtype`, `/addperson`, `/setrotation` — category picker is now hierarchical: global categories shown at the top level; groups with scoped categories appear as navigation buttons; tapping a group shows group-level categories and any topics that have topic-level categories; ⬅️ Back navigates up the tree

## [0.0.26]

### Fixed
- `/addcategory` — `EditMessageText` calls that embedded user-provided strings (group name, topic name) with `ParseMode: Markdown` silently failed when names contained `_`, `*`, or `[`; removed `ParseMode` from all category-flow message edits so the UI always updates

## [0.0.25]

### Fixed
- `/addcategory` — stale Telegram callbacks (`confirm:global`, `topic:`) arriving after bot restart triggered `addCategoryNow` immediately with empty fields; handlers now only act when session is in the expected `StepAdminCatSelectTopic` step, ignoring out-of-sequence callbacks

## [0.0.24]

### Changed
- `/addcategory` flow — group and topic selection moved to the start: command now opens a group picker first, then a topic picker (if the group has registered topics), then proceeds to name → emoji → team key; context header (`🏘️ Group · 📌 Topic`) shown throughout all text-entry steps

### Fixed
- `/addcategory` — registered topics not shown when running the command from DM after a bot restart; `getAllTopics` now loads directly from DB instead of relying on the in-memory group cache
- Missing ❌ Cancel button in topic selection and topic-confirm keyboards

## [0.0.23]

### Added
- 🗑 Clear tag button next to each user in `/users` — skips label input, goes straight to group selection, and clears the tag via `setChatMemberTag` with an empty string

## [0.0.22]

### Added
- User autodiscovery — bot stores `user_id`, username, and display name for every user seen in approved groups; only writes to DB when profile data changes
- `/users` admin command — paginated list of all known users with a **🏷 Set Tag** button per user; tapping a user starts the setlabel flow directly without needing to forward a message

### Fixed
- Set label flow now works from `/users` selection, bypassing the forward-message limitation that blocked tag assignment when bot permissions denied reading user data from forwards

## [0.0.21]

### Fixed
- Set label flow — forwarding a new user message in DM now always restarts the flow, even when a previous session is stuck at the group-selection step

## [0.0.20]

### Fixed
- `/ticket` — removed topic root message guard that incorrectly blocked tickets in groups with a default General topic (where the first message ID equals the thread ID)
- `/addtopic` — group list now reads from DB instead of in-memory cache, so all approved groups are shown regardless of bot restart

## [0.0.19]

### Added
- Message logging — every message (groups + DMs) is appended to a CSV file with columns: timestamp, chat_type, chat_id, chat_title, thread_id, topic_name, user_id, username, first_name, last_name, message_id, text
- Media-only messages logged with placeholders: `[photo]`, `[video]`, `[document]`, `[sticker]`, etc.
- `/export` admin DM command — sends the current CSV as a file attachment, then resets the log
- `MESSAGES_CSV` env var — path for the message log file (default: `messages.csv`)

## [0.0.18]

### Added
- `/categories` admin command — lists all categories with their current scope (global / group / topic); tap any category to change scope or delete it
- Category detail view: Make Global, Group-level, Topic-level, Delete — all inline, no extra messages

## [0.0.17]

### Removed
- `/granttags` command — bot cannot promote itself; manage permissions manually in group settings

### Changed
- README updated: setup instructions, bot permission requirements, member tag flow documented

## [0.0.16]

### Added
- `/granttags` admin command — promotes the bot with `can_manage_tags` permission in the current group (requires "Add New Admins" enabled temporarily)

## [0.0.15]

### Fixed
- Added debug logging to set label flow to trace failures after label input

## [0.0.14]

### Fixed
- Set label flow now shows only approved groups from DB instead of in-memory cache

## [0.0.13]

### Added
- Group approval system — bot auto-registers groups on first message, only works in approved groups
- `/groups` admin command — lists all known groups with approve/disapprove inline buttons, updated in place

### Removed
- `ALLOWED_CHAT_ID` env var — replaced by DB-driven group approval

## [0.0.12]

### Added
- Set Telegram member tag flow: admin forwards a user message to bot in DM → types label (1–16 chars) → selects group → tag set via `setChatMemberTag` (Bot API 9.5)

## [0.0.11]

### Added
- `/setlabel @username <label>` admin command — assigns a display label to a user in the group chat

## [0.0.10]

### Added
- `/ticket` — Added extra metadata in summary and added description.

## [0.0.9]

### Fixed
- `/ticket` — rejects usage when user replies to topic header instead of a real message, with a clear error

## [0.0.8]

### Fixed
- `/ticket` — reporter correctly taken from `replied.From` (person who posted in the group, not forward origin)
- Telegram message links now include thread ID for topic messages (`/c/CHATID/THREADID/MSGID`)

### Added
- Extended `/ticket` debug logging — logs replied message ID, sender, forward status and origin

## [0.0.7]

### Fixed
- `/ticket` — forwarded messages now correctly detect the original sender instead of the forwarder

### Added
- Cancel button (`❌ Cancel`) on category and request type selection keyboards — cancels the flow in-place without sending extra messages

## [0.0.6]

- Fixing github link in `/version` command response

## [0.0.5]

### Added
- `/version` command — shows current bot version and repository link
- Version injected at build time via `-ldflags` from Docker build arg

## [0.0.4]

### Added
- Docker Compose file for running bot from image
- Log rotation limit: 15 MB per file, 3 files max

### Changed
- `/ticket` — now requires replying to a message instead of pasting a link; reporter name and username captured automatically
- Reporter's Telegram account included in Linear issue description for both `/support` and `/ticket`
- CI Docker image tagged as `VERSION-BRANCHNAME` instead of `snapshot`

## [0.0.3]

### Added
- **`/ticket` flow mode** — prompts for the message link interactively instead of requiring it as a command argument
- **`/addtopic` guided flow** — group selection → topic name → topic ID
- **`/topics` and `/rotation` restricted to admins**
- **Auto-create Linear labels** — category and request type labels created in Linear automatically if missing
- **Both category and type applied as Linear labels**
- **Session TTL reaper** — background goroutine evicts abandoned sessions after 30 minutes
- **DB indexes** on `categories`, `support_assignments`, `category_request_types`, `group_topics`

### Changed
- Migration system replaced with `golang-migrate/migrate` — versioned `*.up.sql` files, no custom code
- `/ticket` stores Telegram message link as-is
- `/topics` aggregates all known groups, works correctly from DMs
- `/addcategory` from DM correctly stores target group `chat_id`
- Command registry is single source of truth for routing, help text, and admin-only enforcement
- Help menu shows admin commands only to admins; unauthorized calls blocked at dispatch level

### Fixed
- Linear label GraphQL query used wrong field name (`labels` → `issueLabels`)
- Request type name was empty in created Linear issues
- Threaded Telegram message links (`/c/CHATID/THREADID/MSGID`) parsed incorrectly

## [0.0.2]

### Added
- SQLite persistence for categories, request types, support persons, and topics
- Self-service issue creation (`/support`) — category → request type → title → description → optional media
- Support-assisted ticket creation (`/ticket`) from a Telegram message link
- Daily/weekly support rotation with on-duty assignment (`/rotation`)
- Admin commands: `/addcategory`, `/addtype`, `/addperson`, `/setrotation`, `/setworkhours`, `/addtopic`
- Admin DM support — admins can use all commands via private messages
- Linear label resolution and caching; rich issue descriptions with Telegram metadata

## [0.0.1]

### Added
- Telegram bot with long polling, chat ID whitelist, topic/forum thread support
- `/help` command
- Linear GraphQL integration — team key resolution, issue creation
