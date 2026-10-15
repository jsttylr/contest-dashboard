# Contest Dashboard

> **Disclaimer:** This is an independent hobby project. It is **not affiliated with, endorsed by or supported by** the N1MM Logger+ team or anyone else. All product names belong to their owners and are used only to describe compatibility.
>
> This software was **developed with AI** (Claude, by Anthropic) and has only been tested at one station. **Use it at your own risk.** It comes with no warranty of any kind. Licensed under GPL-3.0.

A live dashboard for ham radio contests. It listens for the UDP broadcasts N1MM Logger+ sends and shows score, rates, operators, radios, band and mode breakdowns, and a map of every contact with great-circle lines from your station. Anyone on your network can watch it in a web browser: a TV in the shack, other operators, or guests at Field Day.

It is a single small program. No Node-RED, no Raspberry Pi, no database to install. It can run on the logging computer itself, on any other computer on the network, or as a Docker container.

## What you see

- **Score, QSOs, mults and points.** The score is the one N1MM calculates and broadcasts, so it is always right for the contest you're in. If N1MM isn't sending score reports, the dashboard shows an estimate (points × mults) and says so.
- **Rates**: last hour, last 10 minutes (as an hourly rate), and the best 60 minutes so far.
- **Map** of every contact, coloured by band, with lines to the 25 most recent. Contacts are placed by grid square when the exchange has one, by state, province or ARRL section in domestic contests, and otherwise by country (from the callsign prefix, using the same country file loggers use). No callsign lookups or internet services are needed for this.
- The map background is a free one that needs no account or API key: Esri's grey map (default) or OpenStreetMap, chosen on the Settings page. The dashboard fetches map pieces itself and saves them, so a map that's been viewed once keeps working without internet.
- **Radios**: frequency, mode, operator, run or search-and-pounce, and a red TX marker while transmitting, for every radio N1MM reports.
- **Contacts over time** by band, **bands and modes** table, **operators** league table, **top countries**, and an **hour by hour** rate sheet.
- **Operators**: click any operator for their own page (run rate, S&P rate, average rate, best hour and best 10 minutes, time on the air, bands, countries, hour by hour), or use **Compare operators** to see everyone side by side, sortable, with the best in each column highlighted. Run and S&P rates count only the time spent doing each, and leave out breaks over 10 minutes.
- A **full-screen map** page (the **Map** button), made for a TV or streaming.

## First run (Windows)

1. Download `contest-dashboard-windows.zip` from the latest release on the [Releases page](https://github.com/jsttylr/contest-dashboard/releases), right-click it, choose **Extract All**, and put the folder somewhere permanent, for example `C:\Ham\ContestDashboard\`.
2. Double-click `contest-dashboard.exe`. Windows may say "Windows protected your PC": click **More info**, then **Run anyway**. (The file isn't code-signed yet.)
3. When Windows Firewall asks, tick **Private networks** and click **Allow**. This lets other computers watch the dashboard and lets N1MM on other computers reach it.
4. Your web browser opens the dashboard at http://127.0.0.1:8060. There is no program window. Double-click the program again any time to reopen the page. To stop it, use **Stop the dashboard program** at the bottom of the Settings page.

To try it before a contest, open **Settings** and click **Start the demo**. It makes up a two-radio contest with a few hours of history and new contacts at a realistic pace. Delete it from the Contests list when you're done.

## Pointing N1MM+ at the dashboard

In N1MM+ open **Config**, then **Configure Ports, Mode Control, Winkey, etc.**, then the **Broadcast Data** tab.

1. Tick **Radio**, **Contacts** and **Score**.
2. In the address box next to each, type the address of the computer running the dashboard and a port, for example `127.0.0.1:12060` when the dashboard runs on the same computer, or `192.168.1.50:12060` for another computer. (Settings shows this computer's address.) You can list more than one destination separated by spaces if another program also needs the data.
3. If the Score tile keeps saying "estimate" instead of "from N1MM" during a contest, check that **Score** is ticked and also look at N1MM's score reporting settings. The dashboard itself never sends anything to the internet.

In a multi-computer N1MM network, set this up on every logging computer.

The **Settings** page shows whether packets are arriving and from where. The dashboard listens on ports 12060, 12061 and 12062 by default and works out what each packet is from its contents, so it doesn't matter which of those ports each type is sent to.

## Looking back at an old contest

Export it from N1MM with **File**, **Export to ADIF**, then use **Import an ADIF log** on the Settings page. Pick it from the contest list at the top of the dashboard.

## Running it with Docker

Docker runs programs in a sealed box called a container, which is handy on a home server, a NAS or a Raspberry Pi that's always on. GitHub builds the container automatically and stores it with the code at `ghcr.io/jsttylr/contest-dashboard`.

On a Linux machine with Docker installed:

```sh
docker run -d --name contest-dashboard --restart unless-stopped \
  --network host \
  -v contest-dashboard-data:/data \
  ghcr.io/jsttylr/contest-dashboard:latest
```

Then open `http://<that machine's address>:8060/` from any browser on your network, and point N1MM at that same address on port 12060.

- `--network host` lets the container hear UDP broadcasts. If you'd rather not use it, replace it with `-p 8060:8060 -p 12060-12062:12060-12062/udp`; that works as long as N1MM sends straight to the machine's address rather than to a broadcast address.
- `-v contest-dashboard-data:/data` keeps settings and contacts when the container is updated.
- To update: `docker pull ghcr.io/jsttylr/contest-dashboard:latest`, then remove and re-run the container. Your data stays in the volume.
- Inside Docker, settings can be changed from any computer on your network (normally only the computer running the dashboard can change them).
- Docker Desktop on Windows can't hear UDP broadcasts well, so on Windows the plain program above is the easier choice.

The image tags are `latest` (newest release), a version number such as `0.1.0`, and `main` (newest build, may be unfinished).

## Updating

Download the new zip and run the new `contest-dashboard.exe`. If an older version is still running, the new one stops it and takes over; your settings and contacts are kept.

## Where things are kept

Settings (`config.json`), the contact log (`contacts.jsonl`) and a log file are kept in:

- Windows: `%AppData%\contest-dashboard\`
- Mac: `~/Library/Application Support/contest-dashboard/`
- Linux: `~/.config/contest-dashboard/`
- Docker: the `/data` volume

The contact log is plain text, one contact per line, written as each contact arrives, so a crash or power cut loses nothing.

## Building from source

Install Go 1.24 or newer, then `go build .` in this folder. `go test ./...` runs the tests.

## Credits

- Country data: AD1C's [country files](https://www.country-files.com/), via `@ham2k/lib-country-files` (MIT).
- Map: [Leaflet](https://leafletjs.com/) (BSD-2-Clause), with the outline from [world-atlas](https://github.com/topojson/world-atlas) / Natural Earth (ISC, public domain). Online map tiles from Esri and OpenStreetMap contributors.
- Charts: [Chart.js](https://www.chartjs.org/) (MIT).
- Inspired by the Node-Red Contesting Dashboard.

## License

GPL-3.0. See [LICENSE](LICENSE).
