# Changelog

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
