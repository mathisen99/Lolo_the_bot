# Lolo IRC Bot

A modular IRC bot with AI-powered conversations, image generation, and extensible commands.

## Features

### AI-Powered Tools
- **AI Conversations** - GPT-5.2 powered responses when mentioned
- **YouTube Search** - Search videos, channels, and read comments
- **Usage Tracking** - Track token usage and costs
- **Web Search** - Real-time information lookup via Brave Search
- **URL Fetching** - Retrieve and analyze web page content
- **Image Generation** - Create images from text prompts (Flux AI, GPT Image 1.5, Nano Banana Pro)
- **Image Editing** - Modify existing images with text instructions
- **Image Analysis** - Describe images, solve visual puzzles, OCR text recognition
- **Image Archiving** - Auto-download images from configured channels to local `img/` folder
- **Python Sandbox** - Secure Python execution in Firecracker microVM (matplotlib, numpy, pandas, etc.)
- **Text Pasting** - Create pastes for long code/text (botbin.net integration)
- **Chat History** - Access conversation context and history
- **Shell Execution** - Run system commands (owner only)
- **Voice Cloning** - Clone voices from audio samples or YouTube clips (CosyVoice2)
- **IRC Commands** - Execute IRC commands via AI (whois, NickServ, ChanServ, channel queries)
- **Source Code Introspection** - AI can read and explain its own source code
- **Agentic Status Reporting** - Real-time status updates for complex tasks
- **Knowledge Base (RAG)** - Learn from PDFs/articles and answer questions about them
- **Video Generation** - Generate short videos from text prompts (Sora 2)

### Bot Management
- **User Memories** - Per-user memories and custom personas that persist across conversations
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
- ffmpeg (for audio/video processing)
- yt-dlp (optional, for voice cloning from YouTube)
- NVIDIA GPU with CUDA (optional, for voice cloning)
- KVM access (optional, for Python sandbox - `/dev/kvm`)
- Docker (optional, for building Python sandbox rootfs)

**API Keys Required:**
- **OpenAI API key** - For AI conversations and reasoning ([platform.openai.com](https://platform.openai.com))
- **Black Forest Labs API key** - For Flux AI image generation/editing ([api.bfl.ml](https://api.bfl.ml))
- **Botbin API key** - For file hosting (images, audio, text) - sign up at [botbin.net](https://botbin.net)
- **Gemini API Key** - For Nano Banana Pro image generation ([ai.google.dev](https://ai.google.dev))
- **Google API Key** - For YouTube Search (enable YouTube Data API v3 on Google Cloud Console)

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

### Voice Cloning Setup (Optional)

Voice cloning requires additional setup for CosyVoice2 and vocal isolation.

**CosyVoice2 Setup:**
```bash
cd CosyVoice
uv venv
source .venv/bin/activate
uv pip install -r requirements.txt

# Download pretrained models (required, ~2GB)
# Models are downloaded automatically on first use, or manually:
python -c "from modelscope import snapshot_download; snapshot_download('iic/CosyVoice2-0.5B', local_dir='pretrained_models/CosyVoice2-0.5B')"
```

**IsolateVoice Setup (for YouTube vocal extraction):**
```bash
cd IsolateVoice
uv venv
source .venv/bin/activate
uv pip install -r requirements.txt
```

### Python Sandbox Setup (Optional)

The Python sandbox uses Firecracker microVMs for secure code execution with no network access.

**Requirements:**
- Linux with KVM access (`/dev/kvm`)
- Docker (for building the rootfs)

**Setup:**
```bash
cd scripts/firecracker/

# Download Firecracker binary and kernel
./setup.sh

# Build Python rootfs with libraries (matplotlib, numpy, pandas, etc.)
sudo ./build-rootfs.sh

# Start the VM (runs in background)
sudo ./start-vm.sh &

# To stop the VM
sudo ./stop-vm.sh
```

**Available libraries:** matplotlib, numpy, pandas, Pillow, graphviz, diagrams, seaborn, scipy, sympy, networkx, plotly

If the VM is not running, Python execution falls back to local (unsandboxed) execution.

### Semantic Search Setup (Optional)

Enable semantic/embedding-based search for chat history using ChromaDB.

```bash
# Initialize ChromaDB with existing chat messages
python3 scripts/migrate_to_chroma.py
```

This creates a vector database for semantic search queries like "what did people say about databases" instead of exact keyword matches.

### API Key Setup

Create a `.env` file in the project root with your API keys:

```bash
# Required for AI conversations and reasoning
OPENAI_API_KEY=sk-your-openai-key-here

# Required for Flux AI image generation/editing
BFL_API_KEY=your-black-forest-labs-key-here

# Required for file hosting (images, audio, text)
BOTBIN_API_KEY=your-botbin-api-key-here

# Required for Nano Banana Pro image generation (Gemini)
GEMINI_API_KEY=your-gemini-api-key-here
```

**Where to get API keys:**
- **OpenAI**: Sign up at [platform.openai.com](https://platform.openai.com) → API Keys
- **Black Forest Labs**: Get free credits at [api.bfl.ml](https://api.bfl.ml) → Sign up
- **Botbin**: Sign up at [botbin.net](https://botbin.net) → Request token from dashboard
- **Gemini**: Get API key at [ai.google.dev](https://ai.google.dev) → Get API key

Alternatively, export them as environment variables:
```bash
export OPENAI_API_KEY="sk-your-openai-key-here"
export BFL_API_KEY="your-black-forest-labs-key-here"
export BOTBIN_API_KEY="your-botbin-api-key-here"
export GEMINI_API_KEY="your-gemini-api-key-here"
export GOOGLE_API_KEY="your-google-api-key-here"
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
| **Python Sandbox** | Run Python code in secure Firecracker VM | "plot sin(x)", "calculate factorial(100)" |
| **Text Pasting** | Create pastes for long content | "paste this code snippet" |
| **User Memories** | Store personal facts and preferences | "remember I like cats" |
| **Shell Execution** | Run system commands (owner only) | "check disk space" |
| **Voice Cloning** | Clone voices and generate speech | "clone this voice and say hello" |
| **YouTube Search** | Search videos and comments | "search youtube for funny cats" |
| **GPT Image 1.5** | High-quality image generation with text | "generate with GPT: a sign saying HELLO" |
| **Nano Banana Pro** | Google's advanced image generation | "generate with gemini: an infographic about AI" |
| **Usage Stats** | Check costs and token usage | "how much have I spent today?" |
| **Bug Report** | Report issues with the bot | "I want to report a bug: image generation fails" |
| **IRC Commands** | Query IRC services and channel info | "who owns nick foobar", "do you have op here" |
| **Source Code** | Read bot's own source code | "how do you handle image generation" |
| **Null Response** | Ask bot to stay silent | "don't respond to this", "shh" |
| **Knowledge Base** | Learn and recall documents | "learn this URL", "what did that paper say about X" |
| **Video Generation** | Generate short videos from prompts | "generate a video of a cat on a beach" |

### Knowledge Base (RAG)

Teach the bot to remember documents (PDFs, articles) and query them later:
```
<you>  lolo: learn this https://arxiv.org/pdf/2511.09030v1
<lolo> Done — I've learned that arXiv PDF. Ask me about it anytime!

<you>  lolo: what have you learned?
<lolo> I've learned 1 source: "Solving a Million-Step LLM Task" https://arxiv.org/pdf/2511.09030v1

<you>  lolo: what does that paper say about error recovery?
<lolo> KB hit: it proposes modular error correction with "red-flagging" unreliable outputs...

<you>  lolo: forget https://arxiv.org/pdf/2511.09030v1
<lolo> Removed from knowledge base.
```

Supports PDFs and web pages. Content is chunked and embedded for semantic search.

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

### User Memories

Store multiple personal facts, preferences, and personas that persist across conversations:
```
<you>  lolo: remember that I like cats
<lolo> Got it! Added memory #1: "User likes cats"

<you>  lolo: remember my favorite color is blue
<lolo> Added memory #2: "User's favorite color is blue"

<you>  lolo: roleplay as a pirate
<lolo> Added memory #3: "Roleplay as a pirate"

<you>  lolo: list my memories
<lolo> [1] ✓ User likes cats | [2] ✓ Favorite color is blue | [3] ✓ Roleplay as pirate

<you>  lolo: forget that I like cats
<lolo> Deleted memory #1

<you>  lolo: disable entry 3
<lolo> Disabled memory #3. It's saved but won't be applied.
```

Owners/admins can manage memories for other users:
```
<admin> lolo: add memory for bob that he speaks French
<lolo>  Done, added memory for bob.
```

### Shell Execution (Owner Only)

The bot owner can run system commands:
```
<owner> lolo: check disk space
<lolo>  /dev/sda1  50G  25G  25G  50% /

<owner> lolo: show memory usage
<lolo>  Mem: 16Gi total, 8Gi used, 8Gi free...

<owner> lolo: run uptime && free -h
<lolo>  up 5 days, 3:42, load: 0.5 | Mem: 16Gi...
```

Non-owners attempting shell commands will be denied.

### Voice Cloning

Clone voices from audio samples or YouTube videos:
```
<you>  lolo: clone this voice https://example.com/voice.mp3 and say "Hello world"
<lolo> https://botbin.net/abc123.mp3

<you>  lolo: use this youtube video https://youtube.com/watch?v=xxx from 1:00 to 1:15 to clone the voice and say "Testing voice clone"
<lolo> https://botbin.net/def456.mp3
```

For YouTube clips, the bot automatically:
1. Downloads the specified time range
2. Isolates vocals (removes music/background using Demucs)
3. Uses the clean voice for cloning

Best results with 5-15 seconds of clear speech. Max YouTube clip: 30 seconds.

### IRC Commands

The bot can execute IRC commands and query channel information:
```
<you>  lolo: who owns the nick foobar?
<lolo> foobar is registered to John, last seen 2 days ago...

<you>  lolo: do you have op in here?
<lolo> No, I don't have op in #channel

<you>  lolo: how many users are in this channel?
<lolo> #channel has 150 users, 3 ops, 12 voiced

<you>  lolo: does alice have voice?
<lolo> Yes, alice is voiced in #channel (+v)

<you>  lolo: find channels about python
<lolo> Found: #python (500 users), #python-offtopic (120 users)...
```

Available queries (all users):
- NickServ INFO, ChanServ INFO, ALIS channel search
- WHOIS, CTCP VERSION/TIME
- Channel user counts, ops list, voiced list, topics
- Check if bot has op/voice, find users across channels

Admin/Owner can also use moderation commands through the AI:
- Kick, ban, quiet users
- Set channel modes and topics
- Request op/voice from ChanServ

### Python Sandbox (Firecracker VM)

Secure Python execution in an isolated microVM with no network access:
```
<you>  lolo: calculate 100 factorial
<lolo> 9332621544394415268169923885626670049071596826...

<you>  lolo: plot sin(x) from 0 to 10
<lolo> Generated: https://botbin.net/abc123.png

<you>  lolo: create a bar chart of sales data [10, 25, 40, 30]
<lolo> https://botbin.net/def456.png
```

Available libraries: matplotlib, numpy, pandas, Pillow, graphviz, diagrams, seaborn, scipy, sympy, networkx, plotly

**Setup (Optional - uses fallback if not configured):**
```bash
cd scripts/firecracker/
./setup.sh                    # Download Firecracker + kernel
sudo ./build-rootfs.sh        # Build Python rootfs (requires Docker)
sudo ./start-vm.sh &          # Start the VM
```

The VM runs persistently and uses vsock for host-guest communication. No network access for security.

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
BOTBIN_API_KEY=your-actual-botbin-key-here
GEMINI_API_KEY=your-actual-gemini-key-here
GOOGLE_API_KEY=your-actual-google-key-here
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

[images]
download_channels = ["#yourchannel"]  # Channels to auto-download images from

[api]
circuit_breaker_threshold = 5  # Failures before circuit opens
max_retries = 3                # Max API retry attempts

### Step 5: Initialize Semantic Search (Optional)

If you have existing chat history and want to enable semantic search:

```bash
# Initialize ChromaDB with existing messages
python3 scripts/migrate_to_chroma.py
```
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
# Enable/disable specific tools
web_search_enabled = true      # Web search via Brave
fetch_url_enabled = true       # URL content retrieval
flux_create_enabled = true     # Image generation (Flux)
flux_edit_enabled = true       # Image editing (Flux)
gpt_image_enabled = true       # High quality image generation (GPT Image 1.5)
gemini_image_enabled = true    # Nano Banana Pro image generation (Gemini)
image_analysis_enabled = true  # Image analysis/OCR
python_exec_enabled = true     # Python code execution
paste_enabled = true           # Text pasting to botbin.net
user_rules_enabled = true      # Per-user memories and rules
chat_history_enabled = true    # Conversation history access
shell_exec_enabled = true      # Shell command execution (owner only)
voice_speak_enabled = true     # Voice cloning/speaking (CosyVoice2)
youtube_search_enabled = true  # YouTube search and stats
usage_stats_enabled = true     # Token usage and cost tracking
report_status_enabled = true   # Agentic status reporting
null_response_enabled = true   # Ability to stay silent
bug_report_enabled = true      # User bug reporting system
irc_command_enabled = true     # IRC commands and channel queries
source_code_enabled = true     # Source code introspection
sora_video_enabled = true      # Video generation (Sora 2)
```

**Note:** Tools requiring missing API keys will be automatically disabled.

### Configuration Files Summary

| File | Purpose | Required |
|------|---------|----------|
| `.env` | API keys (OpenAI, BFL, Botbin, Gemini, Google) | Yes |
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
- Verify `BOTBIN_API_KEY` for file hosting

**Bot not responding to mentions**
- Check Python API is running on port 8000
- Verify OpenAI API key is valid and has credits
- Check `api/config/ai_settings.toml` for disabled tools

**Permission denied errors**
- Use `!verify PASSWORD` via PM (not in channel)
- Check user permission level with `!user list`
- Only owners can add/remove admins

**Voice cloning not working**
- Ensure CosyVoice and IsolateVoice venvs are set up (see Voice Cloning Setup)
- Check `ffmpeg` is installed: `sudo apt install ffmpeg`
- Check `yt-dlp` is installed for YouTube: `sudo apt install yt-dlp`
- Verify CUDA is available for GPU acceleration (CPU works but is slow)
- Check CosyVoice pretrained models are downloaded

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
├── CosyVoice/                   # Voice cloning engine (CosyVoice2)
├── IsolateVoice/                # Vocal isolation (Demucs)
├── .env.example                 # API keys template
├── .env                         # Your API keys (create this)
└── requirements.txt             # Python dependencies
```

## License

MIT

## Contributing

Issues and PRs welcome!
