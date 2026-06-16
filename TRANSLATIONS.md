# Multilingual Support Setup

## Overview
The bot now supports multilingual messages through a translation system. All user-facing messages are moved to YAML translation files.

## Structure

### Translation Files
- `resources/i18n/eng.yaml` — English translations
- `resources/i18n/ru.yaml` — Russian translations

### Translation Package
- `internal/i18n/translations.go` — Translation loader and manager

## Usage

### Loading Translations
The Handler automatically loads English by default on startup (configurable via environment variable):

```go
// Loads language from BOT_LANGUAGE env var, defaults to English
langCode := os.Getenv("BOT_LANGUAGE")
if langCode == "" {
    langCode = "eng"  // Default: English
}

var lang i18n.Lang
switch langCode {
case "eng":
    lang = i18n.English
case "ru":
    lang = i18n.Russian
default:
    lang = i18n.English
}

trans, err := i18n.Load(lang)
h.trans = trans
```

### Setting Language

**Default (English):**
```bash
./bot  # Uses English (eng)
```

**Russian:**
```bash
BOT_LANGUAGE=ru ./bot
```

**English (explicit):**
```bash
BOT_LANGUAGE=eng ./bot
```

### Per-User Language Detection (Future)
To add language detection per user:
```go
// Detect user language from Telegram profile or DB
userLang := getUserLanguage(userID) // returns i18n.Lang
trans := i18nManager.Get(userLang)
// Use trans instead of h.trans
```

### Using Translations in Code
Replace hardcoded strings with translation keys:

```go
// Before
h.sendMessage(ctx, b, msg, "🗂️ Select category for this thread:")

// After
h.sendMessage(ctx, b, msg, h.trans.Thread.SelectCategory)
```

## Message Categories

Translations are organized by feature area:

- **Thread** — Tech thread messages (`thread.go`, `handleThread`, `completeTechThread`, `handleCloseThread`)
- **Ticket** — Support ticket messages (`ticket.go`)
- **Category** — Category management messages (`categories.go`)
- **Person** — Support person messages (`persons.go`)
- **Admin** — Administrative messages (`admin.go`)
- **DNS** — DNS management messages (`dns.go`)
- **Error** — Error messages (used across all files)
- **Common** — Common UI elements
- **OnDuty** — On-call duty messages
- **Rotation** — Rotation management
- **RequestType** — Request type messages
- **Group** — Group messages
- **User** — User lookup messages

## Changes Made

### 1. Thread Creation Flow
- **Removed**: Linear issue link button when thread is created (initial message)
- **Added**: Group and Topic buttons only on creation
- **Kept**: Linear button shown when thread is closed via `/close`

### 2. Translated Files
- `internal/telegram/thread.go` — All messages now use `h.trans.Thread.*`, `h.trans.Error.*`, etc.

### 3. Updated Handler
- Added `trans *i18n.Translations` field
- Loads English translations in `New()` function
- Ready for per-user language detection

## Adding More Translations

### Step 1: Create New Language File
Copy `eng.yaml` to a new language code (e.g., `resources/i18n/kk.yaml` for Kazakh) and translate.

### Step 2: Update i18n Package
Add constant to `internal/i18n/translations.go`:
```go
const (
    English Lang = "eng"
    Russian Lang = "ru"
    Kazakh  Lang = "kk"  // New
)
```

### Step 3: Use in Handler
```go
userLang := detectUserLanguage(userID) // returns i18n.Lang
trans := i18nManager.Get(userLang)
```

## Next Steps

To complete multilingual rollout:

1. **Update remaining files** to use `h.trans`:
   - `ticket.go` — Ticket support messages
   - `categories.go` — Category management
   - `persons.go` — Person management
   - `admin.go` — Admin commands
   - All other telegram handlers

2. **Add language detection**:
   - Store user language preference in database
   - Auto-detect from Telegram profile (`msg.From.LanguageCode`)
   - Add `/language` command to set preference

3. **Add more languages** as needed (Kazakh, etc.)

4. **Button labels** — Some buttons are dynamically built; ensure they all use translations

## Fallback Behavior

- If a translation file fails to load, the handler logs a warning and uses empty translations
- Missing translation keys will return empty strings (should not happen if all keys are used)
- Always falls back to English if a language is not loaded
