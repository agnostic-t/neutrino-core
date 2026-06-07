![[logo]](./assets/logo.png)

English | [Русский](./README_RU.md)

# The Neutrino Project

> The main goal is to ensure open access to information for all

This repository is the core of the project, that is, the basis for all neutrino projects.

The repository consists mainly of interfaces for creating VPN modules, as well as a common client and server (they use all interfaces from the core and accept their implementations as input).

Because of this architecture, creating your own VPN based on neutrino core boils down to implementing the necessary interfaces from the core.

## Modules

There are 6 modules in total:

- `core`: The main client and server, they are responsible for the communication of all other modules with each other
- `transport`: A transport module that sets the method of transmitting information over the network. 
- `obfuscation`: Traffic encryption/obfuscation module. Applies to outgoing and incoming packets.
- `handshake`: A module for transferring the connection target from the client to the server
- `local`: A module for connecting a device to a VPN. Supplies local proxies (for example SOCKS5) and passes the connection target to the handshake module 
- `nmux`: Optional module, needed for multiplexing transport traffic

## Usage

This repository should only be used in combination with the implementation of all 5 modules (that is, excluding `core`, since it is not a module with interfaces).

Repositories with examples of module implementation:

1) obfs: [neutrino-obfs](https://github.com/agnostic-t/neutrino-obfs)
2) handshake: [neutrino-handsh](https://github.com/agnostic-t/neutrino-handsh)
3) local: [neutrino-lproxies](https://github.com/agnostic-t/neutrino-lproxies)
4) transport: [neutrino-transport](https://github.com/agnostic-t/neutrino-transport)
5) nmux: [neutrino-mux](https://github.com/agnostic-t/neutrino-mux)

An example of using all the modules is:
- [Tau](https://github.com/agnostic-t/tau ) a VPN project that makes full use of all the basic implementations (neutrino-...) modules and supports SOCKS5/TUN modes
