![[logo]](./assets/logo.png)

# Neutrino project

> The main task is to ensure open access to information resources for everyone

This module is a core of the project, means that it is a basement for all neutrino-realated products.

Repository includes interfaces and main server/client that are based on these interfaces.

Other neutrino-... repositories consist of implementations of `core`, `local`, `obfuscation` and `transport` modules.

## Modules

Core consists of 4 modules:
 - `core`: Module for generic clients/servers
 - `local`: Module for local proxies on client side (like SOCKS5, HTTP or HTTPS proxy)
 - `obfuscation`: Module for hiding neutrino traffic from IDS/DPI etc. (here goes cryptography too)
 - `transport`: Module for data-transport over network methods (UDP, TCP, ICMP, DNS queries, HTTP-based protos)

## Usage

This repository does not conatin any implementation of VPN based on core, but contains **examples** of how vpn should be written (in exmaples/client and examples/server)

Some of implementations of the VPN on neutrino-core:

- [Tau](https://github.com/agnostic-t/tau): very simple VPN, uses TCP, SOCKS5 and XOBFS
