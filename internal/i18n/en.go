package i18n

var enCatalog = Catalog{
	CLI: CLIText{
		Usage: `filedrop — end-to-end encrypted file transfer

Usage:
  filedrop login                       Sign in (opens a page in your browser)
  filedrop whoami                      Show the current account
  filedrop logout                      Sign out
  filedrop share <file>...             Encrypt, upload and print a link (24 hours by default)
  filedrop share --clipboard           Share the clipboard text as a file
  filedrop save <link | ID[#key]>      Download and decrypt into the current directory
  filedrop list                        List files in the account (ID, size, expiry only)
  filedrop rm <ID>...                  Delete files and free space immediately
  filedrop quota                       Show account storage usage
  filedrop version                     Print the version

Command options:
  filedrop help <command>              Show options for a command, e.g. filedrop help save
`,
		UsageLine:      "Usage: %s",
		OptionsHeader:  "Options:",
		NoSuchCommand:  "No such command: %q",
		UnknownCommand: "Unknown command %q; run filedrop help for usage",
		SeeHelpHint:    "%v (run filedrop help for usage)",
		HelpOnlyOne:    "help accepts one command name; extra arguments: %v",
		SaveOnlyOne:    "save accepts one link; extra arguments: %v",
		Canceled:       "Canceled.",

		SynLogin:   "filedrop login [--no-browser]",
		SynLogout:  "filedrop logout",
		SynWhoami:  "filedrop whoami",
		SynShare:   "filedrop share <file>... | --clipboard [options]",
		SynSave:    "filedrop save <link | ID[#key]> [--out DIR] [--password PW]",
		SynList:    "filedrop list",
		SynRm:      "filedrop rm <ID>...",
		SynQuota:   "filedrop quota",
		SynHelp:    "filedrop help <command>",
		SynVersion: "filedrop version",

		SumLogin:   "Sign in. Opens a browser where you choose GitHub or Google; after you approve, the CLI is signed in.",
		SumLogout:  "Sign out and clear the sign-in saved on this machine. Files already shared are unaffected.",
		SumWhoami:  "Show which account is currently signed in.",
		SumShare:   "Send files and print links you can give to others. Each file gets its own link and is deleted after 24 hours by default.",
		SumSave:    `Download a file from a link. Existing files are never overwritten; duplicates become "name (1).ext".`,
		SumList:    "List unexpired files in the account. Only ID, size and remaining time are shown—file names are encrypted and only you can see them.",
		SumRm:      "Delete files and free their space immediately. Links stop working; files already downloaded are unaffected. Multiple IDs are allowed.",
		SumQuota:   "Show used and available space, and how many files the account has.",
		SumHelp:    "Show detailed usage for a command, e.g. filedrop help save. With no command name, print the overview.",
		SumVersion: "Print the current version.",

		FlagPassword:     "Protect the link with a password",
		FlagMaxDownloads: "Limit the number of downloads (0 = unlimited)",
		FlagCopy:         "Copy the link to the clipboard after upload",
		FlagClipboard:    "Share the text in the clipboard",
		FlagOut:          `Save directory (default: current directory; "-" writes to stdout)`,
		FlagSavePassword: "Password to decrypt a password-protected file",
		FlagConcurrency:  "Number of chunks downloaded in parallel (0 = automatic, default 4)",
		FlagNoBrowser:    "Print the link without opening a browser",

		LoggedIn:        "Signed in.",
		LoggedOut:       "Signed out.",
		NotLoggedIn:     "Not signed in.",
		CurrentAccount:  "Signed in: %s",
		NeedLogin:       "Not signed in; run filedrop login first",
		ShareNeedsLogin: "Sharing requires signing in; run filedrop login first (you can also upload from the web without an account)",

		SetPasswordPrompt:    "Set a password (at least 8 characters):",
		RepeatPasswordPrompt: "Enter it again:",
		PasswordMismatch:     "The passwords do not match",
		PasswordTooShort:     "Password must be at least 8 characters",
		SavePasswordPrompt:   "Password:",
		PasswordHint:         "Do not send the password in the same message as the link.",
		KeyHint:              "The link contains the key; sending it is enough.",

		CopyFailed:           "Could not copy: %s",
		ClipboardReadFailed:  "Could not read the clipboard: %w",
		MaxDownloadsNegative: "--max-downloads cannot be negative",
		ConcurrencyNegative:  "--concurrency cannot be negative",
		Saved:                "Saved: %s",

		NoFiles:    "No unexpired files in this account.",
		ColID:      "ID",
		ColSize:    "SIZE",
		ColExpires: "EXPIRES",

		Deleted:      "Deleted %s",
		DeleteFailed: "%d file(s) could not be deleted",
		QuotaLine:    "%s / %s, %d file(s)",
	},
	Share: ShareText{
		DirectoryNotFile: "Cannot share a directory; choose a file",
		FileTooLarge:     "File exceeds the %s per-file limit",
		ContentTooLarge:  "Content exceeds the %s per-file limit",
		QuotaExceeded:    "Account storage is full: %s of %s used. Delete old files with filedrop rm <ID>, or wait for them to expire",
		QuotaHint:        "%s (run filedrop quota to check usage, or filedrop rm to delete old files)",
		ReadFailed:       "Failed to read %s: %w",
		FileChanged:      "Failed to read %s: the file changed during transfer",
		UploadPartFailed: "Failed to upload a part: %w",
		UploadPartStatus: "Failed to upload a part: HTTP %d %s",
		MissingETag:      "Object storage did not return an ETag (check that the bucket CORS exposes ETag)",
		MissingPartURL:   "Server did not return part URLs",
		ProgressLabel:    "Uploading",
	},
	Filex: FilexText{
		MissingKey:          "This link is missing the key after #, so it cannot be decrypted; ask the sender to resend the complete link",
		EmptyTarget:         "No link or ID to download",
		BadURL:              "Invalid link: %w",
		MissingFileID:       "No file ID found in the link (expected /d/<id>)",
		UnrecognizedTarget:  "Unrecognized link or ID",
		BadKey:              "The key in the link is malformed; check that the link is complete",
		NotFound:            "File does not exist, has expired or was deleted",
		PasswordRequired:    "This file requires a password",
		DownloadUnavailable: "File has expired, was deleted, or reached its download limit",
		SizeMismatch:        "Decrypted size does not match metadata; the file may be incomplete",
		ChunkDownloadFailed: "Chunk %d/%d failed to download: %w",
		ChunkVerifyFailed:   "Chunk %d/%d failed verification: %w",
		StdoutConcurrent:    "Concurrent writes to stdout are not supported",
		HTTPStatus:          "Download failed: HTTP %d",
		TransferFailed:      "Download failed: %w",
		RangeIgnored:        "Object storage ignored the Range request; cannot download in chunks",
		ProgressLabel:       "Downloading",
	},
	Auth: AuthText{
		OpenLink:        "Open the link below in your browser to sign in:",
		DeviceCode:      "Device code %s, valid for %d minutes.",
		BrowserFallback: "Could not open the browser; copy the link above instead.",
		BrowserOpened:   "Opened the browser; waiting for approval…",
		NoToken:         "Server did not return a login token",
		Denied:          "Sign-in request was denied",
		Expired:         "Sign-in request expired; run filedrop login again",
	},
	Config: ConfigText{
		NoConfigDir: "Cannot find the config directory: %w",
		Corrupt:     "Config file is corrupt (%s): %w",
	},
	Crypto: CryptoText{
		WrongPassword:    "Incorrect password",
		BadFileID:        "Invalid file ID",
		MasterKeyLength:  "Master key must be 32 bytes",
		ChunkTooLarge:    "Chunk %d exceeds 8 MiB",
		ChunkNotFull:     "Chunk %d is not the last chunk, so it must be 8 MiB",
		ChunkTooShort:    "Chunk %d ciphertext is shorter than the authentication tag",
		ChunkAuthFailed:  "Chunk %d failed verification; the file may be tampered with or the link incomplete",
		MetaTooLong:      "Metadata is too long",
		MetaBadBase64:    "Metadata is not valid base64url",
		MetaTooShort:     "Metadata ciphertext is too short",
		MetaAuthFailed:   "Could not decrypt metadata: wrong key or tampered file",
		MetaBadJSON:      "Metadata is not valid JSON",
		MetaVersion:      "Unsupported metadata version",
		BadName:          "Invalid file name",
		BadSize:          "Invalid file size",
		BadType:          "Invalid content type",
		BadChunkSize:     "Invalid chunk size",
		BadChunkCount:    "Invalid chunk count",
		PasswordTooShort: "Password must be at least 8 characters",
		BadWrapSalt:      "Invalid password-wrapper salt",
		BadWrapCipher:    "Invalid password-wrapper ciphertext",
		BadWrapLength:    "Invalid password-wrapper ciphertext length",
		UnsupportedKDF:   "Unsupported password derivation algorithm %q",
		WeakKDF:          "Password derivation iteration count is too low",
	},
	API: APIText{
		ConnectFailed: "Cannot connect to %s: %w",
		DecodeFailed:  "Cannot parse the response: %w",
	},
	Term: TermText{
		EmptyInput:      "No input",
		Expired:         "expired",
		LessThanMinute:  "less than a minute",
		MinutesLeft:     "%d min left",
		HoursLeft:       "%d h left",
		DaysLeft:        "%d days left",
		NoClipboardTool: "No clipboard tool available",
		ClipboardEmpty:  "Clipboard is empty (text only)",
	},
}
