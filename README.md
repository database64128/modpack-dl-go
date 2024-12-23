# Minecraft Modpack Downloader

[![Test](https://github.com/database64128/modpack-dl-go/actions/workflows/test.yml/badge.svg)](https://github.com/database64128/modpack-dl-go/actions/workflows/test.yml)
[![Release](https://github.com/database64128/modpack-dl-go/actions/workflows/release.yml/badge.svg)](https://github.com/database64128/modpack-dl-go/actions/workflows/release.yml)
[![AUR version](https://img.shields.io/aur/version/modpack-dl-go-git?label=modpack-dl-go-git)](https://aur.archlinux.org/packages/modpack-dl-go-git)

⚒️⏬ Minecraft modpack downloader written in Go.

## Usage

```bash
# Display help.
modpack-dl-go -h

# Retrieve and print information about a modpack and its latest version.
modpack-dl-go -modpackID 120

# Retrieve and print information about a modpack and the specified version.
modpack-dl-go -modpackID 120 -versionID 11334

# Download the latest modpack client to the specified directory.
modpack-dl-go -modpackID 120 -clientPath /tmp/modpack-dl-go/client

# Download the latest modpack server to the specified directory.
modpack-dl-go -modpackID 120 -serverPath /tmp/modpack-dl-go/server

# Download the latest modpack client and server to the specified directories.
modpack-dl-go -modpackID 120 -clientPath /tmp/modpack-dl-go/client -serverPath /tmp/modpack-dl-go/server

# Upgrade an existing modpack installation to the latest version.
modpack-dl-go -modpackID 120 -clientPath /tmp/modpack-dl-go/client -serverPath /tmp/modpack-dl-go/server -migrateFromPath /tmp/modpack-dl-go/old

# Same as above, but copy files instead of moving them.
modpack-dl-go -modpackID 120 -clientPath /tmp/modpack-dl-go/client -serverPath /tmp/modpack-dl-go/server -migrateFromPath /tmp/modpack-dl-go/old -preserveMigrationSource
```

## License

[GPLv3](LICENSE)
