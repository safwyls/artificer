package main

// Choosing a folder, the way the rest of the desktop does it.
//
// The web UI had to grow its own folder browser — a /api/browse route,
// a tree of buttons, a "saveish" heuristic to point at the likely next
// click — because a page in a browser cannot ask the OS where a folder
// is. That whole apparatus is the clearest thing the native app
// deletes: `dialog.Directory()` opens the shell's own picker, with the
// player's real drives, their pinned folders, their muscle memory, and
// their ability to paste a path into it.
//
// The /api/browse route stays exactly where it was — it is frozen
// surface, and the browser page still needs it. This window simply never
// calls it.

import (
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	sysdialog "github.com/sqweek/dialog"
)

// folderPickerButton is the "…" beside a path entry. It opens the OS
// folder dialog starting where the entry already points, and writes the
// choice back.
func (u *ui) folderPickerButton(entry *widget.Entry) *widget.Button {
	b := widget.NewButtonWithIcon("", theme.FolderOpenIcon(), func() {
		go func() {
			d := sysdialog.Directory().Title("Choose the save folder")
			if start := trim(entry.Text); start != "" {
				d = d.SetStartDir(start)
			}
			dir, err := d.Browse()
			if err != nil {
				// Cancelling is the ordinary outcome and is not worth
				// reporting; anything else is, quietly.
				if err != sysdialog.ErrCancelled {
					u.say(err.Error(), true)
				}
				return
			}
			setEntryText(entry, dir)
		}()
	})
	return b
}

// filePickerButton is the same for a launch target, which is a file
// (an .exe, a shortcut) rather than a folder.
func (u *ui) filePickerButton(entry *widget.Entry) *widget.Button {
	return widget.NewButtonWithIcon("", theme.FileIcon(), func() {
		go func() {
			d := sysdialog.File().Title("Choose what starts this game")
			path, err := d.Load()
			if err != nil {
				if err != sysdialog.ErrCancelled {
					u.say(err.Error(), true)
				}
				return
			}
			setEntryText(entry, path)
		}()
	})
}
