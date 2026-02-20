# Installation

## Binary Downloads

Download the latest pre-built binary for your platform from the [GitHub Releases page](https://github.com/AbdelazizMoustafa10m/Raven/releases).

=== "macOS (Apple Silicon)"

    ```bash
    curl -Lo raven.tar.gz https://github.com/AbdelazizMoustafa10m/Raven/releases/latest/download/raven_<VERSION>_darwin_arm64.tar.gz
    tar -xzf raven.tar.gz
    sudo install -m 755 raven /usr/local/bin/raven
    ```

=== "macOS (Intel)"

    ```bash
    curl -Lo raven.tar.gz https://github.com/AbdelazizMoustafa10m/Raven/releases/latest/download/raven_<VERSION>_darwin_amd64.tar.gz
    tar -xzf raven.tar.gz
    sudo install -m 755 raven /usr/local/bin/raven
    ```

=== "Linux (x86-64)"

    ```bash
    curl -Lo raven.tar.gz https://github.com/AbdelazizMoustafa10m/Raven/releases/latest/download/raven_<VERSION>_linux_amd64.tar.gz
    tar -xzf raven.tar.gz
    sudo install -m 755 raven /usr/local/bin/raven
    ```

=== "Linux (ARM64)"

    ```bash
    curl -Lo raven.tar.gz https://github.com/AbdelazizMoustafa10m/Raven/releases/latest/download/raven_<VERSION>_linux_arm64.tar.gz
    tar -xzf raven.tar.gz
    sudo install -m 755 raven /usr/local/bin/raven
    ```

Verify the checksum against `checksums.txt` included in the release:

```bash
sha256sum -c checksums.txt --ignore-missing
```

!!! note "Windows"
    Download the `.zip` from the releases page. For the best experience on Windows, consider running Raven under [WSL](https://learn.microsoft.com/en-us/windows/wsl/).

## From Source

Requires Go 1.24 or later.

```bash
git clone https://github.com/AbdelazizMoustafa10m/Raven.git
cd Raven
CGO_ENABLED=0 go build -o raven ./cmd/raven
sudo install -m 755 raven /usr/local/bin/raven
```

## Homebrew (coming soon)

```bash
# Not yet available. Track https://github.com/AbdelazizMoustafa10m/Raven/issues
# brew install abdelazizmoustafa10m/tap/raven
```

## Verify Installation

```bash
raven version
```
