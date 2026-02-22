Require micasa

Output docs/static/images/extraction.gif

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
Env PS1 ""
Env MICASA_EXTRACTION_MODEL "qwen3:0.6b"

Hide
# Stage a test PDF in a temp dir so the file picker only shows one file.
Type "mkdir -p /tmp/demo && cp internal/extract/testdata/scanned-invoice.pdf /tmp/demo/"
Enter
Sleep 0.3s
Type "cd /tmp/demo"
Enter
Sleep 0.3s
# Disable XON/XOFF flow control so Ctrl+S reaches the app.
Type "stty -ixon"
Enter
Sleep 0.5s
Type "exec micasa --demo"
Enter
Sleep 5s
# Dismiss dashboard.
Type "D"
Sleep 2s
# Jump to Docs tab.
Type "F"
Sleep 2s
Show

Sleep 1s

# Enter Edit mode, then magic add a new document (Shift+A).
Type "i"
Sleep 0.5s
Type "A"
Sleep 2s

# File picker opens in /tmp/demo with only scanned-invoice.pdf.
# Select it with Enter.
Enter
Sleep 1s

# Save -- triggers extraction overlay.
Ctrl+S
Sleep 2s

# Extraction overlay should now be visible with live progress.
# Wait for OCR and LLM to complete.
Sleep 15s

# Accept results.
Type "a"
Sleep 2s

Hide
Ctrl+Q
Sleep 1s
