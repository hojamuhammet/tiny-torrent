# torrent-client

Tiny BitTorrent client written in Golang

## Install

```sh
git clone github.com/hojamuhammet/tiny-torrent.git
```

## Usage
Try by downloading Debian!

```sh
torrent-client debian-10.2.0-amd64-netinst.iso.torrent debian.iso
```


## Limitations
* Only supports `.torrent` files (no magnet links)
* Only supports HTTP trackers
* Does not support multi-file torrents
* Strictly leeches (does not support uploading pieces)