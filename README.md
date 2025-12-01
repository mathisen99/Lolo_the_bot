# Lolo IRC Bot

A modular IRC bot with AI-powered conversations, image generation, and extensible commands.

## Features

### AI-Powered Tools
- **AI Conversations** - GPT-5.1 powered responses when mentioned
- **Web Search** - Real-time information lookup via Brave Search
- **URL Fetching** - Retrieve and analyze web page content
- **Image Generation** - Create images from text prompts (Flux AI)
- **Image Editing** - Modify existing images with text instructions
- **Image Analysis** - Describe images, solve visual puzzles, OCR text recognition
- **Python Code Execution** - Run Python code snippets safely
- **Text Pasting** - Create pastes for long code/text (bpa.st integration)
- **Chat History** - Access conversation context and history

### Bot Management
- **User Rules** - Per-user custom personas that persist across conversations
- **User Management** - Permission levels (owner, admin, normal, ignored)
- **IRC Formatting** - AI can use colors, bold, italic, and other IRC formatting
- **Moderation** - Kick, ban, mute, and channel management commands
- **Audit Logging** - Track all administrative actions
- **Rate Limiting** - Prevent spam with per-user cooldowns

## Quick Start

### Prerequisites

- Go 1.21+
- Python 3.10+
- [uv](https://docs.astral.sh/uv/getting-started/installation/) (Python package manager)

**API Keys Required:**
- **OpenAI API key** - For AI conversations and reasoning ([platform.openai.com](https://platform.openai.com))
- **Black Forest Labs API key** - For Flux AI image generation/editing ([api.bfl.ml](https://api.bfl.ml))
- **Freeimage.host API key** - For image hosting (free at [freeimage.host/api](https://freeimage.host/api))

### Installation

```bash
# Clone and enter directory
git clone https://github.com/mathisen99/Lolo_the_bot.git
cd Lolo_the_bot

# Build Go bot
go mod download
go build -o lolo cmd/bot/main.go

# Set up Python environment
uv venv
source .venv/bin/activate
uv pip install -r requirements.txt
```

### API Key Setup

Create a `.env` file in the project root with your API keys:

```bash
# Required for AI conversations and reasoning
OPENAI_API_KEY=sk-your-openai-key-here

# Required for Flux AI image generation/editing
BFL_API_KEY=your-black-forest-labs-key-here

# Required for image hosting (free tier available)
FREEIMAGE_API_KEY=your-freeimage-key-here
```

**Where to get API keys:**
- **OpenAI**: Sign up at [platform.openai.com](https://platform.openai.com) → API Keys
- **Black Forest Labs**: Get free credits at [api.bfl.ml](https://api.bfl.ml) → Sign up
- **Freeimage.host**: Free API key at [freeimage.host/api](https://freeimage.host/api) → Register

Alternatively, export them as environment variables:
```bash
export OPENAI_API_KEY="sk-your-openai-key-here"
export BFL_API_KEY="your-black-forest-labs-key-here"
export FREEIMAGE_API_KEY="your-freeimage-key-here"
```

### Running

**Terminal 1 - Python API:**
```bash
source .venv/bin/activate
uvicorn api.main:app --host 0.0.0.0 --port 8000
```

**Terminal 2 - Go Bot:**
```bash
./lolo
```

On first run, set an admin password when prompted, then verify ownership via PM:
```
/msg Lolo !verify YOUR_PASSWORD
```

## Usage

### AI Conversations

Mention the bot to chat:
```
<you>  lolo: what's the weather in Tokyo?
<lolo> Currently Tokyo is 22°C with partly cloudy skies...

<you>  lolo: generate an image of a sunset over mountains
<lolo> https://iili.io/abc123.png

<you>  lolo: what's in this image? https://example.com/photo.jpg
<lolo> The image shows a golden retriever playing in a park...

<you>  lolo: edit this image to make the sky purple: https://example.com/sunset.jpg
<lolo> https://iili.io/edited123.png

<you>  lolo: run this python code: print("hello world")
<lolo> hello world

<you>  lolo: search for latest news about AI
<lolo> Here are the latest AI developments I found...
```

### Available AI Tools

The bot has access to these tools when mentioned:

| Tool | Description | Example Usage |
|------|-------------|---------------|
| **Web Search** | Search the web for current information | "search for weather in Tokyo" |
| **URL Fetch** | Retrieve and analyze web page content | "what's on this page: https://example.com" |
| **Image Generation** | Create images from text descriptions | "generate a sunset over mountains" |
| **Image Editing** | Modify existing images with instructions | "edit this image to add a rainbow" |
| **Image Analysis** | Analyze, describe, or extract text from images | "what's in this image?" |
| **Python Execution** | Run Python code snippets | "calculate fibonacci sequence in python" |
| **Text Pasting** | Create pastes for long content | "paste this code snippet" |
| **User Rules** | Manage personal AI behavior rules | "roleplay as a pirate" |

### IRC Formatting

The AI can use colors and formatting in responses. It uses custom tags that get converted to IRC control codes:

```
<you>  lolo: write hello world in python with syntax highlighting
<lolo> print("Hello, world!")  ← keywords in blue, strings in green
```

Supported formatting:
- `<BOLD>`, `<ITALIC>`, `<UNDERLINE>`, `<STRIKE>`, `<MONO>`
- `<COLOR fg="03">green text</COLOR>` (colors 00-15)
- `<COLOR fg="04" bg="01">red on black</COLOR>`
- Nesting supported: `<BOLD><COLOR fg="04">bold red</COLOR></BOLD>`

### User Rules (Custom Personas)

Set personal rules that change how the bot responds to you:
```
<you>  lolo: roleplay as a pirate
<lolo> Aye aye! I'll be speakin' like a pirate for ye from now on!

<you>  lolo: what rules do I have?
<lolo> Yer rules: Always speak as a friendly pirate...

<you>  lolo: turn off my roleplay
<lolo> Rules disabled, but saved for later.

<you>  lolo: clear my rules
<lolo> Your rules have been cleared.
```

Owners/admins can manage rules for other users:
```
<admin> lolo: set rules for bob to respond in French
<lolo>  Done, bob will now receive responses in French.
```

### Commands

| Command | Description | Permission |
|---------|-------------|------------|
| `!help` | List commands | All |
| `!ping` | Pong | All |
| `!version` | Bot version | All |
| `!uptime` | Bot uptime | All |
| `!user add/remove/list` | Manage users | Admin+ |
| `!kick/ban/mute` | Moderation | Admin+ |
| `!join/part` | Channel management | Owner |
| `!quit` | Shutdown bot | Owner |

Use `!help <command>` for detailed help.

## Configuration

### Step 1: Copy Example Files

The repository includes `.example` files that you need to copy and customize:

```bash
# Copy the example config files
cp .env.example .env
cp config/bot.toml.example config/bot.toml
```

### Step 2: Configure API Keys (.env)

Edit `.env` and add your API keys:

```bash
OPENAI_API_KEY=sk-your-actual-key-here
BFL_API_KEY=your-actual-bfl-key-here
FREEIMAGE_API_KEY=your-actual-freeimage-key-here
```

### Step 3: Configure IRC Settings (config/bot.toml)

Edit `config/bot.toml` to customize your bot's IRC connection:

```toml
[server]
address = "irc.libera.chat"    # IRC server
port = 6697                     # Port (6697 for TLS)
nickname = "YourBotName"        # Bot's nickname
username = "yourbot"            # Bot's username
realname = "Your IRC Bot"       # Bot's real name

[auth]
sasl_username = "YourBotName"   # For registered nicks (optional)
sasl_password = ""              # SASL password (optional)
nickserv_password = ""          # NickServ password (optional)

[bot]
command_prefix = "!"            # Command prefix (e.g., !help)
channels = ["#yourchannel"]     # Channels to join on startup
```

### Step 4: Customize AI Personality (api/config/ai_settings.toml)

Edit `api/config/ai_settings.toml` to personalize your bot. Key things to customize:

**Bot Identity** - Find and edit these lines in the `[system_prompt]` section:
```toml
# Change the creator name
"Mathisen created me"  →  "YourName created me"

# Change the bot's self-description
"I'm am Lolo of course!"  →  "I'm YourBotName!"
```

**AI Behavior:**
```toml
[model]
reasoning_effort = "low"  # low/medium/high - affects AI thinking depth
verbosity = "low"         # low/medium/high - response detail level
```

**Enable/Disable Tools:**
```toml
[tools]
web_search_enabled = true      # Web search via Brave
fetch_url_enabled = true       # URL content retrieval
flux_create_enabled = true     # Image generation (requires BFL_API_KEY)
flux_edit_enabled = true       # Image editing (requires BFL_API_KEY)
image_analysis_enabled = true  # Image analysis/OCR
python_exec_enabled = true     # Python code execution
paste_enabled = true           # Text pasting to bpa.st
user_rules_enabled = true      # Per-user custom rules
chat_history_enabled = true    # Conversation history access
```

**Note:** Tools requiring missing API keys will be automatically disabled.

### Configuration Files Summary

| File | Purpose | Required |
|------|---------|----------|
| `.env` | API keys (OpenAI, BFL, Freeimage) | Yes |
| `config/bot.toml` | IRC server, nickname, channels, auth | Yes |
| `api/config/ai_settings.toml` | AI personality, tools, system prompt | Optional (has defaults) |

## Troubleshooting

### Common Issues

**"Tool disabled due to missing API key"**
- Check your `.env` file has the correct API key
- Restart the Python API after adding keys
- Verify API key format (OpenAI keys start with `sk-`)

**Image generation not working**
- Ensure `BFL_API_KEY` is set correctly
- Check Black Forest Labs account has credits
- Verify `FREEIMAGE_API_KEY` for image hosting

**Bot not responding to mentions**
- Check Python API is running on port 8000
- Verify OpenAI API key is valid and has credits
- Check `api/config/ai_settings.toml` for disabled tools

**Permission denied errors**
- Use `!verify PASSWORD` via PM (not in channel)
- Check user permission level with `!user list`
- Only owners can add/remove admins

## Project Structure

```
lolo/
├── cmd/bot/main.go              # Bot entry point
├── internal/                    # Go bot internals
├── api/                         # Python API
│   ├── ai/                      # AI client & config
│   ├── tools/                   # AI tools (search, images, rules)
│   ├── commands/                # Custom commands
│   └── config/
│       └── ai_settings.toml     # AI personality & tool settings
├── config/
│   ├── bot.toml.example         # IRC config template (copy to bot.toml)
│   └── bot.toml                 # Your IRC configuration (create this)
├── data/                        # Database & logs (auto-created)
├── .env.example                 # API keys template
├── .env                         # Your API keys (create this)
└── requirements.txt             # Python dependencies
```

## License

MIT

## Contributing

Issues and PRs welcome!
