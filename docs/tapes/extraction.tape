Require micasa

Output docs/static/images/extraction.gif

Set Shell bash
Set FontFamily "Hack Nerd Font"
Set FontSize 32
Set Width 2400
Set Height 1600
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
# Jump to Docs tab (backward from first tab wraps to last).
Type "F"
Sleep 1s
Show

Sleep 1s

# Add a new document.
Type "a"
Sleep 2s

# Tab to the file field and type a path to sample PDF.
Tab
Sleep 0.3s
Type "internal/extract/testdata/scanned-invoice.pdf"
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
