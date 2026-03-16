# Changelog

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
