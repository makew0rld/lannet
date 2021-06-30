# lannet

Lannet creates a little web on the LAN. It runs a fileserver daemon in the background, and hosts a homepage that links to other lannet servers on the same network.

Here's the view of the homepage, with one other lannet server on the network:

![homepage](screenshots/homepage.png)

By default the server serves files from the `~/lannet` folder, but any root path can be set.

I see lannet as being useful in a classroom environment. Many students are on the same LAN, developing websites, and with lannet they have immediate access to a web server with no config, and can easily check out the websites of their classmates at any time, with no need to type in IP addresses.

```
lannet - a little web on the LAN

Commands:

lannet
    Start the daemon if needed, and open the homepage.

lannet version
    View the version information of lannet.

lannet stop
    Stop the daemon if running

lannet root my/path
    Change the webserver root from ~/lannet to my/path

lannet name [new-name]
    View the current name, or set it to a new one
```

Download binaries for lannet on [releases page](https://github.com/makeworld-the-better-one/lannet/releases).

## Building from source

**Requirements:**
- Go 1.16 or later
- GNU Make

Please note the Makefile does not intend to support Windows, and so there may be issues.


```shell
git clone https://github.com/makeworld-the-better-one/lannet
cd lannet
# git checkout v1.2.3 # Optionally pin to a specific version instead of the latest commit
make # Might be gmake on macOS
sudo make install # If you want to install the binary for all users
```

Because you installed with the Makefile, running `lannet version` will tell you exactly what commit the binary was built from.

## License

Lannet is licensed under the GPLv3. See [LICENSE](LICENSE) for details.
