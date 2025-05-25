# torrent-client

Tiny BitTorrent client written in Golang

## Install

```sh
git clone github.com/hojamuhammet/tiny-torrent.git
```

## Usage
Try by downloading Arch Linux!

```sh
tiny-torrent ./torrentfile/test_torrent/archlinux-2019.12.01-x86_64.iso.torrent archlinux.iso
```


## Limitations
* Only supports `.torrent` files (no magnet links)
* Only supports HTTP trackers
* Does not support multi-file torrents
* Strictly leeches (does not support uploading pieces)
