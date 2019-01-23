# AIRGOAT
Airgoat is a music bot that uses [youtube-dl](https://rg3.github.io/youtube-dl/). Airgoat utilizes the [discordgo](https://github.com/bwmarrin/discordgo) library, a free and open source library. Airgoat requires Go 1.4 or higher.

## Usage
Airgoat has two components, a bot client that handles the playing of loyal bees, and a web server that implements OAuth2 and stats. Once added to your server, Airgoat can be summoned by running `!bees`.


### Running the Bot

**First install the bot (assuming Airgoat is added to your GOPATH):**
```
go get bot
go install bot
```
 **Then run the following command:**

```
bot -r "localhost:6379" -t "MY_BOT_ACCOUNT_TOKEN" -o OWNER_ID
```

### Running the Web Server

First install the webserver: `go get webserver` and `go install webserver` then run the bot using:
```
webserver -r "localhost:6379" -i MY_APPLICATION_ID -s 'MY_APPLICATION_SECRET"
```

Note, the webserver requires a redis instance to track statistics

## Thanks
Thanks to the discord devs and the original [Airhorn Bot](github.com/hammerandchisel/airhornbot) devs.
