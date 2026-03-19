# Changelog

## [0.0.26]

<!-- Prepare for next release: remove this line and write your release notes -->


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
