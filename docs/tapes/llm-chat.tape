Require micasa

Output docs/static/videos/llm-chat.webm

Set Shell bash
Set FontFamily "Hack Nerd Font"
Set FontSize 32
Set Width 2400
Set Height 1200
Set Padding 20
Set Theme "Dracula"
Set CursorBlink false
Set TypingSpeed 0

Env NO_COLOR ""
Env TERM "xterm-256color"
Env COLORTERM "truecolor"
Env COLORFGBG "15;0"
Env MICASA_CHAT_LLM_MODEL "mistral-small3.2:24b"
Env PS1 ""

Hide
Type "exec micasa demo"
Enter
Sleep 5s
# Dismiss dashboard.
Type "D"
Sleep 2s
Show

Sleep 0.5s

# Open the chat overlay with @.
Type "@"
Sleep 1.5s

# Type a question.
Set TypingSpeed 40ms
Type "How many projects are on hold right now?"
Set TypingSpeed 0
Sleep 0.5s

# Submit the question.
Enter

# Wait for the LLM to stream its response.
Sleep 8s

# Type a follow-up question.
Set TypingSpeed 40ms
Type "Ok, of those, which has the highest budget?"
Set TypingSpeed 0
Sleep 0.5s

# Submit the follow-up.
Enter

# Wait for the second response.
Sleep 8s

# Dismiss the overlay.
Escape
Sleep 1.5s

Hide
Ctrl+Q
Sleep 1s
