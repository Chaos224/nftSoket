package main

import (
	"errors"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func createMainWindow(a fyne.App) fyne.Window {
	w := a.NewWindow("NFT Client")
	w.Resize(fyne.NewSize(400, 300))

	// Elemente UI pentru Login
	usernameEntry := widget.NewEntry()
	usernameEntry.SetPlaceHolder("Username")

	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("Password")

	loginButton := widget.NewButton("Login", func() {
		username := usernameEntry.Text
		password := passwordEntry.Text

		if username == "" || password == "" {
			dialog.ShowError(errors.New("Username and Password are required!"), w)
			return
		}

		// Apelăm funcția de login
		message, userDir, ipfsHash := login(username, password)
		if message == "Login successful" {
			dialog.ShowInformation("Success", "Logged in successfully!", w)

			// Montare disc virtual
			mountPoint := "Z:\\"
			if err := mountVirtualDrive(mountPoint, ipfsHash, getServerConfig()); err != nil {
				dialog.ShowError(fmt.Errorf("Failed to mount virtual drive: %v", err), w)
				return
			}
			dialog.ShowInformation("Success", "Virtual drive mounted successfully!", w)

			// Sincronizare fișiere (opțional)
			go func() {
				if err := startSyncService(mountPoint, userDir); err != nil {
					dialog.ShowError(fmt.Errorf("Synchronization failed: %v", err), w)
				}
			}()
		} else {
			dialog.ShowError(errors.New(message), w)
		}
	})

	// Buton pentru Configurare
	configButton := widget.NewButton("Configuration", func() {
		showConfigWindow(a)
	})

	// Layout principal
	content := container.NewVBox(
		widget.NewLabel("Login"),
		usernameEntry,
		passwordEntry,
		loginButton,
		configButton,
	)

	w.SetContent(content)
	return w
}

func showConfigWindow(a fyne.App) {
	w := a.NewWindow("Configuration")
	w.Resize(fyne.NewSize(400, 200))

	serverEntry := widget.NewEntry()
	serverEntry.SetText(getServerConfig()) // Citim adresa serverului din configurație

	// Salvare configurare
	saveButton := widget.NewButton("Save", func() {
		saveServerConfig(serverEntry.Text)
		dialog.ShowInformation("Success", "Configuration saved!", w)
	})

	w.SetContent(container.NewVBox(
		widget.NewLabel("Server Address"),
		serverEntry,
		saveButton,
	))
	w.Show()
}
