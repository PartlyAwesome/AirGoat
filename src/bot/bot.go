package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/bwmarrin/discordgo"
	"github.com/dustin/go-humanize"
	redis "gopkg.in/redis.v3"
)

var (
	// discordgo session
	discord *discordgo.Session

	// Redis client connection (used for stats)
	rcli *redis.Client

	// Map of Guild id's to *Play channels, used for queuing and rate-limiting guilds
	queues  = make(map[string]chan *Play)
	skipped = make(map[string]bool)
	caching = make(map[string]bool)

	// gifPosting toggle for gifPost
	gifPosting  = make(map[string]bool)
	memeTimeout = make(map[string]time.Duration)
	lastMeme    = make(map[string]time.Time)
	memeVoice   = make(map[string]bool)

	// BITRATE - Bitrate
	BITRATE = 128
	// MAXQSIZE - Max queue size
	MAXQSIZE = 9999

	// YTAPIKEY - Youtube API Key
	YTAPIKEY string

	// OWNER - Owner
	OWNER string
)

const (
	// SUCCESS - success
	SUCCESS = "success"
)

// Play represents an individual use of the !airhorn command
type Play struct {
	GuildID   string
	ChannelID string
	UserID    string
	Sound     *Sound

	// The next play to occur after this, only used for chaining sounds like anotha
	Next *Play

	// If true, this was a forced play using a specific airhorn sound name
	Forced bool
}

// SoundCollection - Collection of sounds
type SoundCollection struct {
	Prefix    string
	Commands  []string
	Sounds    []*Sound
	ChainWith *SoundCollection

	soundRange int
}

// Sound represents a sound clip
type Sound struct {
	Name string

	// Weight adjust how likely it is this song will play, higher = more likely
	Weight int

	// Delay (in milliseconds) for the bot to wait before sending the disconnect request
	PartDelay int

	// Buffer to store encoded PCM packets
	buffer [][]byte
}

// AIRHORN - Array of all the sounds we have
var AIRHORN = &SoundCollection{
	Prefix: "airhorn",
	Commands: []string{
		"!airhorn",
	},
	Sounds: []*Sound{
		createSound("default", 1000, 250),
		createSound("reverb", 800, 250),
		createSound("spam", 800, 0),
		createSound("tripletap", 800, 250),
		createSound("fourtap", 800, 250),
		createSound("distant", 500, 250),
		createSound("echo", 500, 250),
		createSound("clownfull", 250, 250),
		createSound("clownshort", 250, 250),
		createSound("clownspam", 250, 0),
		createSound("highfartlong", 200, 250),
		createSound("highfartshort", 200, 250),
		createSound("midshort", 100, 250),
		createSound("truck", 10, 250),
	},
}

// KHALED - DJ Khaled
var KHALED = &SoundCollection{
	Prefix:    "another",
	ChainWith: AIRHORN,
	Commands: []string{
		"!anotha",
		"!anothaone",
	},
	Sounds: []*Sound{
		createSound("one", 1, 250),
		createSound("one_classic", 1, 250),
		createSound("one_echo", 1, 250),
	},
}

// CENA - JOHN CENA!!!!!
var CENA = &SoundCollection{
	Prefix: "jc",
	Commands: []string{
		"!johncena",
		"!cena",
	},
	Sounds: []*Sound{
		createSound("airhorn", 1, 250),
		createSound("echo", 1, 250),
		createSound("full", 1, 250),
		createSound("jc", 1, 250),
		createSound("nameis", 1, 250),
		createSound("spam", 1, 250),
	},
}

// ETHAN - H3H3
var ETHAN = &SoundCollection{
	Prefix: "ethan",
	Commands: []string{
		"!ethan",
		"!eb",
		"!ethanbradberry",
		"!h3h3",
	},
	Sounds: []*Sound{
		createSound("areyou_classic", 100, 250),
		createSound("areyou_condensed", 100, 250),
		createSound("areyou_crazy", 100, 250),
		createSound("areyou_ethan", 100, 250),
		createSound("classic", 100, 250),
		createSound("echo", 100, 250),
		createSound("high", 100, 250),
		createSound("slowandlow", 100, 250),
		createSound("cuts", 30, 250),
		createSound("beat", 30, 250),
		createSound("sodiepop", 1, 250),
	},
}

// COW GOES MOO
var COW = &SoundCollection{
	Prefix: "cow",
	Commands: []string{
		"!stan",
		"!stanislav",
	},
	Sounds: []*Sound{
		createSound("herd", 10, 250),
		createSound("moo", 10, 250),
		createSound("x3", 1, 250),
	},
}

// BIRTHDAY - HAPPY BIRTHDAY TO YOU
var BIRTHDAY = &SoundCollection{
	Prefix: "birthday",
	Commands: []string{
		"!birthday",
		"!bday",
	},
	Sounds: []*Sound{
		createSound("horn", 50, 250),
		createSound("horn3", 30, 250),
		createSound("sadhorn", 25, 250),
		createSound("weakhorn", 25, 250),
	},
}

// WOW - WOW
var WOW = &SoundCollection{
	Prefix: "wow",
	Commands: []string{
		"!wowthatscool",
		"!wtc",
		"!wow",
	},
	Sounds: []*Sound{
		createSound("thatscool", 50, 250),
		createSound("wow", 100, 250),
	},
}

// BEES BEES, THEY'RE EVERYWHERE
var BEES = &SoundCollection{
	Prefix: "bees",
	Commands: []string{
		"!bees",
	},
	Sounds: []*Sound{
		createSound("bees", 100, 250),
		createSound("remastered", 100, 250),
		createSound("beedrills", 10, 250),
		createSound("temmie", 10, 250),
		createSound("too", 50, 250),
		createSound("junkie", 25, 250),
		createSound("dammit", 25, 250),
	},
}

// NGAHHH - Angry Fish
var NGAHHH = &SoundCollection{
	Prefix: "ngah",
	Commands: []string{
		"!ngahhh",
		"!ngah",
		"ngahhh",
	},
	Sounds: []*Sound{
		createSound("normal", 100, 250),
		createSound("evil", 10, 250),
		createSound("evil_slide", 10, 250),
		createSound("fast", 50, 250),
		createSound("faster", 50, 250),
		createSound("turbo", 40, 250),
		createSound("turboer", 30, 250),
		createSound("turboest", 20, 250),
		createSound("slow", 40, 250),
		createSound("turboester", 15, 250),
		createSound("turbostar", 10, 250),
		createSound("full", 1, 250),
		createSound("soj", 1, 250),
	},
}

// CANCER - LET'S GET RIGHT INTO THE CANCER
var CANCER = &SoundCollection{
	Prefix: "meme",
	Commands: []string{
		"!cancer",
		"!memes",
		"!dankmemes",
		"!maymays",
		"!dankmaymays",
	},
	Sounds: []*Sound{
		createSound("everythingnomegalo", 10, 250),
		createSound("everything", 10, 250),
		createSound("news", 100, 250),
		createSound("illegal", 100, 250),
		createSound("banestar", 10, 250),
		createSound("keemstar", 10, 250),
		createSound("allstar", 10, 250),
		createSound("noneblackhole", 10, 250),
		createSound("stopcoming", 10, 250),
	},
}

func createEmptySC() *SoundCollection {
	return &SoundCollection{}
}

// COLLECTIONS - Set of collections
var COLLECTIONS = []*SoundCollection{
	AIRHORN,
	KHALED,
	CENA,
	ETHAN,
	COW,
	BIRTHDAY,
	WOW,
	BEES,
	NGAHHH,
	CANCER,
}

// Create a Sound struct
func createSound(Name string, Weight int, PartDelay int) *Sound {
	return &Sound{
		Name:      Name,
		Weight:    Weight,
		PartDelay: PartDelay,
		buffer:    make([][]byte, 0),
	}
}

// Load entire collection
func (sc *SoundCollection) Load() {
	for _, sound := range sc.Sounds {
		sc.soundRange += sound.Weight
		sound.Load(sc)
	}
}

// Random sound from this collection
func (sc *SoundCollection) Random() *Sound {
	var (
		i      int
		number = randomRange(0, sc.soundRange)
	)

	for _, sound := range sc.Sounds {
		i += sound.Weight

		if number < i {
			return sound
		}
	}
	return nil
}

// LoadNow - Modification of Load
func (s *Sound) LoadNow() error {
	path := fmt.Sprintf("audio/%v", s.Name)
	file, err := os.Open(path)

	if err != nil {
		fmt.Println("error opening dca file :", err)
		return err
	}

	var opuslen int16

	for {
		// read opus frame length from dca file
		err = binary.Read(file, binary.LittleEndian, &opuslen)

		// If this is the end of the file, just return
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		}

		if err != nil {
			fmt.Println("error reading from dca file :", err)
			return err
		}

		// read encoded pcm from dca file
		InBuf := make([]byte, opuslen)
		err = binary.Read(file, binary.LittleEndian, &InBuf)

		// Should not be any end of file errors
		if err != nil {
			fmt.Println("error reading from dca file :", err)
			return err
		}

		// append encoded pcm data to the buffer
		s.buffer = append(s.buffer, InBuf)
	}
}

// Unload this sound
func (s *Sound) Unload() {
	s.buffer = nil
}

func (s *Sound) isLoaded() bool {
	if len(s.buffer) != 0 {
		return true
	}
	return false
}

// Load this sound
func (s *Sound) Load(c *SoundCollection) error {
	path := fmt.Sprintf("audio/%v_%v.dca", c.Prefix, s.Name)

	file, err := os.Open(path)

	if err != nil {
		fmt.Println("error opening dca file :", err)
		return err
	}

	var opuslen int16

	for {
		// read opus frame length from dca file
		err = binary.Read(file, binary.LittleEndian, &opuslen)

		// If this is the end of the file, just return
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		}

		if err != nil {
			fmt.Println("error reading from dca file :", err)
			return err
		}

		// read encoded pcm from dca file
		InBuf := make([]byte, opuslen)
		err = binary.Read(file, binary.LittleEndian, &InBuf)

		// Should not be any end of file errors
		if err != nil {
			fmt.Println("error reading from dca file :", err)
			return err
		}

		// append encoded pcm data to the buffer
		s.buffer = append(s.buffer, InBuf)
	}
}

// PlayFile - Plays file
func (s *Sound) PlayFile(vc *discordgo.VoiceConnection, file string) {
	ffmpeg := exec.Command("ffmpeg", "-i", "audio/"+file, "-f", "s16le", "-ar", "48000", "-ac", "2", "pipe:1")
	ffmpegout, err := ffmpeg.StdoutPipe()
	if err != nil {
		log.Println("ffmpeg StdoutPipe err:", err)
		return
	}
	ffmpegbuf := bufio.NewReaderSize(ffmpegout, 16384)

	dca := exec.Command("dca", "-raw", "-i", "pipe:0")
	dca.Stdin = ffmpegbuf
	dcaout, err := dca.StdoutPipe()
	if err != nil {
		log.Println("dca StdoutPipe err:", err)
		return
	}
	dcabuf := bufio.NewReaderSize(dcaout, 16384)

	err = ffmpeg.Start()
	if err != nil {
		log.Println("ffmpeg Start err:", err)
		return
	}
	defer func() {
		go ffmpeg.Wait()
	}()

	err = dca.Start()
	if err != nil {
		log.Println("dca Start err:", err)
		return
	}
	defer func() {
		go dca.Wait()
	}()

	// header "buffer"
	var opuslen int16

	// Send "speaking" packet over the voice websocket
	vc.Speaking(true)
	// Send not "speaking" packet over the websocket when we finish
	defer vc.Speaking(false)

	for {
		// read dca opus length header
		err = binary.Read(dcabuf, binary.LittleEndian, &opuslen)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return
		}
		if err != nil {
			log.Println("read opus length from dca err:", err)
			return
		}

		// read opus data from dca
		opus := make([]byte, opuslen)
		err = binary.Read(dcabuf, binary.LittleEndian, &opus)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return
		}
		if err != nil {
			log.Println("read opus from dca err:", err)
			return
		}

		if skipped[vc.GuildID] {
			ffmpeg.Process.Kill()
			dca.Process.Kill()
			return
		}

		// Send received PCM to the sendPCM channel
		vc.OpusSend <- opus
	}
}

// PlayStream - Plays stream
func (s *Sound) PlayStream(vc *discordgo.VoiceConnection, stream string) {
	log.Info(stream)

	if caching[vc.GuildID] {
		go streamDownload(stream)
	}

	format := "bestaudio"
	if strings.Contains(stream, "youtube.com") || strings.Contains(stream, "youtu.be") {
		format = "mp4"
	}

	ytdl := exec.Command("youtube-dl", "-v", "-f", format, "-o", "-", stream)
	ytdlout, err := ytdl.StdoutPipe()
	if err != nil {
		log.Println("ytdl StdoutPipe err:", err)
		return
	}
	ytdlbuf := bufio.NewReaderSize(ytdlout, 16384)

	ffmpeg := exec.Command("ffmpeg", "-i", "pipe:0", "-f", "s16le", "-ar", "48000", "-ac", "2", "pipe:1")
	ffmpeg.Stdin = ytdlbuf
	ffmpegout, err := ffmpeg.StdoutPipe()
	if err != nil {
		log.Println("ffmpeg StdoutPipe err:", err)
		return
	}
	ffmpegbuf := bufio.NewReaderSize(ffmpegout, 16384)

	dca := exec.Command("dca", "-raw", "-i", "pipe:0")
	dca.Stdin = ffmpegbuf
	dcaout, err := dca.StdoutPipe()
	if err != nil {
		log.Println("dca StdoutPipe err:", err)
		return
	}
	dcabuf := bufio.NewReaderSize(dcaout, 16384)

	err = ytdl.Start()
	if err != nil {
		log.Println("ytdl Start err:", err)
		return
	}
	defer func() {
		go ytdl.Wait()
	}()

	err = ffmpeg.Start()
	if err != nil {
		log.Println("ffmpeg Start err:", err)
		return
	}
	defer func() {
		go ffmpeg.Wait()
	}()

	err = dca.Start()
	if err != nil {
		log.Println("dca Start err:", err)
		return
	}
	defer func() {
		go dca.Wait()
	}()

	// header "buffer"
	var opuslen int16

	// Send "speaking" packet over the voice websocket
	vc.Speaking(true)
	// Send not "speaking" packet over the websocket when we finish
	defer vc.Speaking(false)

	for {
		// read dca opus length header
		err = binary.Read(dcabuf, binary.LittleEndian, &opuslen)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return
		}
		if err != nil {
			log.Println("read opus length from dca err:", err)
			return
		}

		// read opus data from dca
		opus := make([]byte, opuslen)
		err = binary.Read(dcabuf, binary.LittleEndian, &opus)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return
		}
		if err != nil {
			log.Println("read opus from dca err:", err)
			return
		}

		if skipped[vc.GuildID] {
			ytdl.Process.Kill()
			ffmpeg.Process.Kill()
			dca.Process.Kill()
			return
		}

		// Send received PCM to the sendPCM channel
		vc.OpusSend <- opus
	}
}

func streamDownload(stream string, name ...string) string {
	id, err := getIDFromLink(stream)
	log.Info(stream)
	if err != nil {
		return "Invalid link"
	}

	if isLive(id) {
		return "Video is live"
	}

	var dcaName string
	if name != nil {
		dcaName = name[0]
	} else {
		dcaName = getDCAfromLink(stream)
	}

	format := "bestaudio"
	if strings.Contains(stream, "youtube.com") || strings.Contains(stream, "youtu.be") {
		format = "mp4"
	}

	ytdl := exec.Command("youtube-dl", "-v", "-f", format, "-o", "-", stream)
	ytdlout, err := ytdl.StdoutPipe()
	if err != nil {
		log.Println("ytdl StdoutPipe err:", err)
		return "youtube-dl StdoutPipe error"
	}
	ytdlbuf := bufio.NewReaderSize(ytdlout, 16384)

	ffmpeg := exec.Command("ffmpeg", "-i", "pipe:0", "-f", "s16le", "-ar", "48000", "-ac", "2", "pipe:1")
	ffmpeg.Stdin = ytdlbuf
	ffmpegout, err := ffmpeg.StdoutPipe()
	if err != nil {
		log.Println("ffmpeg StdoutPipe err:", err)
		return "ffmpeg StdoutPipe error"
	}
	ffmpegbuf := bufio.NewReaderSize(ffmpegout, 16384)

	dca := exec.Command("dca", "-raw", "-i", "pipe:0")
	dca.Stdin = ffmpegbuf
	outfile, err := os.Create("audio/" + dcaName)
	if err != nil {
		log.Println("file creation err:", err)
		return "file creation error"
	}
	defer outfile.Close()
	dca.Stdout = outfile

	err = ytdl.Start()
	if err != nil {
		log.Println("ytdl Start err:", err)
		return "youtube-dl error"
	}
	defer func() {
		go ytdl.Wait()
	}()

	err = ffmpeg.Start()
	if err != nil {
		log.Println("ffmpeg Start err:", err)
		return "ffmpeg error"
	}
	defer func() {
		go ffmpeg.Wait()
	}()

	err = dca.Start()
	if err != nil {
		log.Println("dca Start err:", err)
		return "dca error"
	}
	defer dca.Wait()

	return SUCCESS
}

// Play this sound over the specified VoiceConnection
func (s *Sound) Play(vc *discordgo.VoiceConnection) {
	nameType := strings.Split(s.Name, "@")
	if len(nameType) > 1 {
		if nameType[1] == "stream" {
			s.PlayStream(vc, nameType[0])
			return
		} else if nameType[1] == "file" {
			s.PlayFile(vc, nameType[0])
			return
		}
	}
	vc.Speaking(true)
	defer vc.Speaking(false)

	for _, buff := range s.buffer {
		if skipped[vc.GuildID] {
			return
		}
		vc.OpusSend <- buff
	}
}

// NEVER RENAME THIS FUNCTION, EVER.
// IF WE WANT TO PROCESS VOTES MAKE A NEW FUNCTION
// CALL IT votes OR SOMETHING
func skip(g *discordgo.Guild) {
	skipped[g.ID] = true
}

// Attempts to find the current users voice channel inside a given guild
func getCurrentVoiceChannel(user *discordgo.User, guild *discordgo.Guild) *discordgo.Channel {
	for _, vs := range guild.VoiceStates {
		if vs.UserID == user.ID {
			channel, _ := discord.State.Channel(vs.ChannelID)
			return channel
		}
	}
	return nil
}

// Returns a random integer between min and max
func randomRange(min, max int) int {
	rand.Seed(time.Now().UTC().UnixNano())
	return rand.Intn(max-min) + min
}

// Prepares a play
func createPlay(user *discordgo.User, guild *discordgo.Guild, coll *SoundCollection, sound *Sound) *Play {
	// Grab the users voice channel
	channel := getCurrentVoiceChannel(user, guild)
	if channel == nil {
		log.WithFields(log.Fields{
			"user":  user.ID,
			"guild": guild.ID,
		}).Warning("Failed to find channel to play sound in")
		return nil
	}

	// Create the play
	play := &Play{
		GuildID:   guild.ID,
		ChannelID: channel.ID,
		UserID:    user.ID,
		Sound:     sound,
		Forced:    true,
	}

	// If we didn't get passed a manual sound, generate a random one
	if play.Sound == nil {
		play.Sound = coll.Random()
		play.Forced = false
	}

	// If the collection is a chained one, set the next sound
	if coll.ChainWith != nil {
		play.Next = &Play{
			GuildID:   play.GuildID,
			ChannelID: play.ChannelID,
			UserID:    play.UserID,
			Sound:     coll.ChainWith.Random(),
			Forced:    play.Forced,
		}
	}

	return play
}

func listQueue(s *discordgo.Session, m *discordgo.MessageCreate, g *discordgo.Guild) {
	_, exists := queues[g.ID]
	if exists {
		s.ChannelMessageSend(m.ChannelID, strconv.Itoa(len(queues[g.ID])))
	}
}

// Prepares and enqueues a play into the ratelimit/buffer guild queue
func enqueuePlay(user *discordgo.User, guild *discordgo.Guild, coll *SoundCollection, sound *Sound, s ...*discordgo.Session) {
	play := createPlay(user, guild, coll, sound)
	if play == nil {
		return
	}

	// Check if we already have a connection to this guild
	//   yes, this isn't threadsafe, but its "OK" 99% of the time
	_, exists := queues[guild.ID]

	if exists {
		if len(queues[guild.ID]) < MAXQSIZE {
			queues[guild.ID] <- play
		}
	} else {
		queues[guild.ID] = make(chan *Play, MAXQSIZE)
		if s != nil {
			playSound(play, nil, s[0])
		} else {
			playSound(play, nil)
		}
	}
}

func trackSoundStats(play *Play) {
	if rcli == nil {
		return
	}

	_, err := rcli.Pipelined(func(pipe *redis.Pipeline) error {
		var baseChar string

		if play.Forced {
			baseChar = "f"
		} else {
			baseChar = "a"
		}

		base := fmt.Sprintf("airhorn:%s", baseChar)
		pipe.Incr("airhorn:total")
		pipe.Incr(fmt.Sprintf("%s:total", base))
		pipe.Incr(fmt.Sprintf("%s:sound:%s", base, play.Sound.Name))
		pipe.Incr(fmt.Sprintf("%s:user:%s:sound:%s", base, play.UserID, play.Sound.Name))
		pipe.Incr(fmt.Sprintf("%s:guild:%s:sound:%s", base, play.GuildID, play.Sound.Name))
		pipe.Incr(fmt.Sprintf("%s:guild:%s:chan:%s:sound:%s", base, play.GuildID, play.ChannelID, play.Sound.Name))
		pipe.SAdd(fmt.Sprintf("%s:users", base), play.UserID)
		pipe.SAdd(fmt.Sprintf("%s:guilds", base), play.GuildID)
		pipe.SAdd(fmt.Sprintf("%s:channels", base), play.ChannelID)
		return nil
	})

	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Warning("Failed to track stats in redis")
	}
}

func ytDCAtoLink(ytDCA string) string {
	split := strings.SplitN(ytDCA, "_", 2)
	if split[0] != "yt" {
		return ""
	}
	idsplit := strings.Split(split[1], ".")
	return "youtu.be/" + idsplit[0]
}

// Play a sound
func playSound(play *Play, vc *discordgo.VoiceConnection, s ...*discordgo.Session) (err error) {
	log.WithFields(log.Fields{
		"play": play,
	}).Info("Playing sound")

	skipped[play.GuildID] = false

	loaded := play.Sound.isLoaded()

	if vc == nil {
		vc, err = discord.ChannelVoiceJoin(play.GuildID, play.ChannelID, false, false)
		// vc.Receive = false
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("Failed to play sound")
			delete(queues, play.GuildID)
			return err
		}
	}

	// If we need to change channels, do that now
	if vc.ChannelID != play.ChannelID {
		vc.ChangeChannel(play.ChannelID, false, false)
		time.Sleep(time.Millisecond * 125)
	}

	// Track stats for this play in redis
	go trackSoundStats(play)

	// Sleep for a specified amount of time before playing the sound
	time.Sleep(time.Millisecond * 32)

	// Play the sound
	if len(s) > 0 {
		link := ytDCAtoLink(play.Sound.Name)
		if link != "" {
			s[0].UpdateStatus(0, link)
		} else {
			s[0].UpdateStatus(0, play.Sound.Name)
		}
	}

	if !loaded {
		play.Sound.LoadNow()
	}
	play.Sound.Play(vc)
	//Will wait till next song is done to do this shit
	log.Info("Played song, advance queue")
	//Put shit here to advance the "fake" queue text
	//vc.GuildID
	advanceQueueList(vc.GuildID)
	if !loaded {
		play.Sound.Unload()
	}

	// If this is chained, play the chained sound
	if play.Next != nil {
		playSound(play.Next, vc)
	}

	// If there is another song in the queue, recurse and play that
	if len(queues[play.GuildID]) > 0 {
		nextPlay := <-queues[play.GuildID]
		if s != nil {
			playSound(nextPlay, vc, s[0])
		} else {
			playSound(nextPlay, vc)
		}
		return nil
	}

	// If the queue is empty, delete it
	time.Sleep(time.Millisecond * time.Duration(play.Sound.PartDelay))
	delete(queues, play.GuildID)
	vc.Disconnect()
	if len(s) > 0 {
		s[0].UpdateStatus(0, "Nothing")
	}
	return nil
}

func onReady(s *discordgo.Session, event *discordgo.Ready) {
	log.Info("Recieved READY payload")
	s.UpdateStatus(0, "Nothing")
}

func onGuildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {
	if event.Guild.Unavailable || queues[event.Guild.ID] == nil {
		return
	}

	initServerSettings(event.Guild.ID)

	for _, channel := range event.Guild.Channels {
		if channel.ID == event.Guild.ID {
			s.ChannelMessageSend(channel.ID, "**AIRGOAT READY TO BEES. TYPE `!BEES` WHILE IN A VOICE CHANNEL TO BEES**")
		}
	}
}

func scontains(key string, options ...string) bool {
	for _, item := range options {
		if item == key {
			return true
		}
	}
	return false
}

func calculateAirhornsPerSecond(cid string) {
	current, _ := strconv.Atoi(rcli.Get("airhorn:a:total").Val())
	time.Sleep(time.Second * 10)
	latest, _ := strconv.Atoi(rcli.Get("airhorn:a:total").Val())

	discord.ChannelMessageSend(cid, fmt.Sprintf("Current APS: %v", (float64(latest-current))/10.0))
}

func displayBotStats(cid string) {
	stats := runtime.MemStats{}
	runtime.ReadMemStats(&stats)

	users := 0
	for _, guild := range discord.State.Ready.Guilds {
		users += len(guild.Members)
	}

	w := &tabwriter.Writer{}
	buf := &bytes.Buffer{}

	w.Init(buf, 0, 4, 0, ' ', 0)
	fmt.Fprintf(w, "```\n")
	fmt.Fprintf(w, "Discordgo: \t%s\n", discordgo.VERSION)
	fmt.Fprintf(w, "Go: \t%s\n", runtime.Version())
	fmt.Fprintf(w, "Memory: \t%s / %s (%s total allocated)\n", humanize.Bytes(stats.Alloc), humanize.Bytes(stats.Sys), humanize.Bytes(stats.TotalAlloc))
	fmt.Fprintf(w, "Tasks: \t%d\n", runtime.NumGoroutine())
	fmt.Fprintf(w, "Servers: \t%d\n", len(discord.State.Ready.Guilds))
	fmt.Fprintf(w, "Users: \t%d\n", users)
	fmt.Fprintf(w, "```\n")
	w.Flush()
	discord.ChannelMessageSend(cid, buf.String())
}

func utilSumRedisKeys(keys []string) int {
	//results := make([]*redis.StringCmd, 0) linting says this is a good change but don't know for sure
	var results []*redis.StringCmd

	rcli.Pipelined(func(pipe *redis.Pipeline) error {
		for _, key := range keys {
			results = append(results, pipe.Get(key))
		}
		return nil
	})

	var total int
	for _, i := range results {
		t, _ := strconv.Atoi(i.Val())
		total += t
	}

	return total
}

func displayUserStats(cid, uid string) {
	keys, err := rcli.Keys(fmt.Sprintf("airhorn:*:user:%s:sound:*", uid)).Result()
	if err != nil {
		return
	}

	totalAirhorns := utilSumRedisKeys(keys)
	discord.ChannelMessageSend(cid, fmt.Sprintf("Total Airhorns: %v", totalAirhorns))
}

func displayServerStats(cid, sid string) {
	keys, err := rcli.Keys(fmt.Sprintf("airhorn:*:guild:%s:sound:*", sid)).Result()
	if err != nil {
		return
	}

	totalAirhorns := utilSumRedisKeys(keys)
	discord.ChannelMessageSend(cid, fmt.Sprintf("Total Airhorns: %v", totalAirhorns))
}

func utilGetMentioned(s *discordgo.Session, m *discordgo.MessageCreate) *discordgo.User {
	for _, mention := range m.Mentions {
		if mention.ID != s.State.Ready.User.ID {
			return mention
		}
	}
	return nil
}

func getYtIDFromLink(link string) (string, error) {
	id := ""
	if strings.Contains(link, "youtube.com/watch?v=") {
		splitlink := strings.SplitAfter(link, "=")
		after := splitlink[1]
		splitafter := strings.Split(after, "&")
		id = splitafter[0]
	} else if strings.Contains(link, "youtu.be/") {
		splitlink := strings.SplitAfter(link, "/")
		var after string
		if strings.Contains(link, "://") {
			after = splitlink[3]
		} else {
			after = splitlink[1]
		}
		splitafter := strings.Split(after, "&")
		id = splitafter[0]
	}
	if len(id) > 11 {
		id = id[0:11]
	}
	return id, nil
}

func getIDFromLink(link string) (string, error) {
	if strings.Contains(link, "youtube.com/watch?v=") || strings.Contains(link, "youtu.be/") {
		return getYtIDFromLink(link)
	}
	_, id, _, _, err := getInfoFromLink(link)
	if err != nil {
		return "", err
	}
	return id, nil
}

//func for getting specific info from video link
//this is in order of retrieval
//0 = title
//1 = id
//2 = duration
//3 = latency
func getInfoPartFromLink(link string, part int) (string, error) {
	if part == 1 {
		return getIDFromLink(link)
	}

	title, _, duration, latency, err := getInfoFromLink(link)
	if err != nil {
		return "", err
	}

	switch part {
	case 0:
		return title, nil
	case 2:
		return duration, nil
	case 3:
		return latency, nil
	default:
		return "", errors.New("Invalid part")
	}
}

//this function uses the YouTube API to find the information rather than using youtube-dl
func getYtInfoFromLink(link string) (title, id, duration, latency string, err error) {
	start := time.Now()
	id, err = getIDFromLink(link)
	if err != nil {
		return "", "", "", "", err
	}
	apiPage := urlToString(ytIDtoAPIurl(id) + "&fields=items(snippet(title),contentDetails(duration))&part=snippet,contentDetails")
	title, err = filterYtAPIresponse(apiPage, "title")
	if err != nil {
		return "", "", "", "", err
	}
	duration, err = filterYtAPIresponse(apiPage, "duration")
	if err != nil {
		return "", "", "", "", err
	}
	return title, id, duration, time.Since(start).String(), nil
}

//REALLY SLOW
//fetches video information using youtube-dl in this order
//0 = title
//1 = id
//2 = duration
//3 = latency
func getInfoFromLink(link string) (title, id, duration, latency string, err error) {
	if strings.Contains(link, "youtube.com/watch?v=") || strings.Contains(link, "youtu.be/") && YTAPIKEY != "" {
		return getYtInfoFromLink(link)
	}

	start := time.Now()
	//info := []string{"Name", "Id", "Duration", "Lat"}
	cmd := exec.Command("youtube-dl", "--get-title", "--get-id", "--get-duration", link)
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	if err != nil {
		return "", "", "", "", err
	}
	info := strings.Split(out.String(), "\n")
	return info[0], info[1], info[2], time.Since(start).String(), nil
}

func returnStringOrError(s string, err error) string {
	if err != nil {
		return err.Error()
	}
	return s
}

func playDCA(s *discordgo.Session, m *discordgo.MessageCreate, g *discordgo.Guild, dca string, silent bool) {
	qm := "Queued: " + dca
	link := ytDCAtoLink(dca)
	dcaSplit := strings.SplitN(dca, "_", 2)
	if link != "" {
		qm = "Queued: " + returnStringOrError(getInfoPartFromLink(link, 0))
	} else if dcaSplit[0] == "tag" {
		dcaSplit = strings.Split(dcaSplit[1], ".")
		qm = "Queued tag: " + dcaSplit[0]
	}
	if !silent {
		s.ChannelMessageSend(m.ChannelID, qm)
	}

	go enqueuePlay(m.Author, g, createEmptySC(), createSound(dca, 1, 250), s)
}

func isDCA(possDCA string) bool {
	split := strings.Split(possDCA, ".")
	if len(split) == 2 && split[1] == "dca" {
		return true
	}
	return false
}

func play(s *discordgo.Session, m *discordgo.MessageCreate, g *discordgo.Guild, toPlay string, silent bool) {

	if isDCA(toPlay) {
		playDCA(s, m, g, toPlay, silent)
		return
	}
	if !silent {
		go s.ChannelMessageSend(m.ChannelID, "Queued: "+returnStringOrError(getInfoPartFromLink(toPlay, 0)))
	}
	go enqueuePlay(m.Author, g, createEmptySC(), createSound(toPlay+"@stream", 1, 250), s)
}

func playFile(s *discordgo.Session, m *discordgo.MessageCreate, g *discordgo.Guild, file string) {
	go enqueuePlay(m.Author, g, createEmptySC(), createSound(file, 1, 250), s)
}

func getDCAfromLink(link string) string {
	return getPrefixFromLink(link) + returnStringOrError(getIDFromLink(link)) + ".dca"
}

func getPrefixFromLink(link string) string {
	prefix := "url_"
	if strings.Contains(link, "youtube.com") || strings.Contains(link, "youtu.be") {
		prefix = "yt_"
	} else if strings.Contains(link, "soundcloud.com") {
		prefix = "sc_"
	}
	return prefix
}

func ytTimeFormat(time string) string {
	w := ""
	d := ""
	h := ""
	m := ""
	s := ""

	curr := strings.Split(time, "P")
	curr = strings.Split(curr[1], "T")
	p := curr[0]
	t := curr[1]

	if p != "" {
		if strings.Contains(p, "W") {
			wp := strings.Split(p, "W")
			w = wp[0] + "w "
			p = wp[1]
		}
		if strings.Contains(p, "D") {
			dp := strings.Split(p, "D")
			d = dp[0] + "d "
			//p = dp[1]
		}
	}
	if strings.Contains(t, "H") {
		hp := strings.Split(t, "H")
		h = hp[0] + "h "
		t = hp[1]
	}
	if strings.Contains(t, "M") {
		mp := strings.Split(t, "M")
		m = mp[0] + "m "
		t = mp[1]
	}
	if strings.Contains(t, "S") {
		sp := strings.Split(t, "S")
		s = sp[0] + "s"
		//t = sp[1]
	}
	return w + d + h + m + s
}

func timeFormat(time string) string {
	if strings.Contains(time, "P") {
		return ytTimeFormat(time)
	}

	// Hours : Minutes : Seconds
	var timeformat string
	//timed := []string{"00", "00", "00"}
	timed := strings.Split(time, ":")
	switch count := strings.Count(time, ":"); count {
	case 0:
		timeformat = timed[0] + "s"
	case 1:
		timeformat = timed[0] + "m " + timed[1] + "s"
	case 2:
		timeformat = timed[0] + "h " + timed[1] + "m"
	default:
		timeformat = "YT Timer Parse Error"
	}
	return timeformat
}

func fileExists(fileName string) bool {
	path := fmt.Sprintf("audio/%v", fileName)
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return false
}

func delDCA(DCA string) string {
	if !fileExists(DCA) {
		return "File doesn't exist"
	}
	rmerr := os.Remove("audio/" + DCA)
	if rmerr != nil {
		log.Info(rmerr)
		return "The file is a persistant bastard."
	}
	return "File removed"
}

func delTag(tag string) string {
	return delDCA("tag_" + tag + ".dca")
}

func delLink(link string) string {
	return delDCA(getDCAfromLink(link))
}

func playTag(s *discordgo.Session, m *discordgo.MessageCreate, g *discordgo.Guild, tag string) {
	tagDCA := "tag_" + tag + ".dca"

	if fileExists(tagDCA) {
		playDCA(s, m, g, tagDCA, false)
		return
	}

	s.ChannelMessageSend(m.ChannelID, tag+" doesn't exist")
}

func tagLink(s *discordgo.Session, m *discordgo.MessageCreate, g *discordgo.Guild, tag string, link string) {
	tagDCA := "tag_" + tag + ".dca"

	if fileExists(tagDCA) {
		s.ChannelMessageSend(m.ChannelID, tag+" already exists")
		return
	}

	s.ChannelMessageSend(m.ChannelID, "Downloading tag: "+tag)
	result := streamDownload(link, tagDCA)
	if result != SUCCESS {
		s.ChannelMessageSend(m.ChannelID, "Failed to create tag, error: "+result)
	} else {
		s.ChannelMessageSend(m.ChannelID, tag+" created")
	}
}

func cleanYTLink(link string) string {
	if strings.Contains(link, "youtube.com") || strings.Contains(link, "youtu.be") {
		return "youtu.be/" + returnStringOrError(getYtIDFromLink(link))
	}
	return link
}

func playLink(s *discordgo.Session, m *discordgo.MessageCreate, g *discordgo.Guild, link string, silent bool) {
	//addToQueueList(g.ID, link)
	link = cleanYTLink(link)
	linkDCA := getDCAfromLink(link)
	if fileExists(linkDCA) {
		playDCA(s, m, g, linkDCA, silent)
	} else {
		play(s, m, g, link, silent)
	}
}

// TODO: This should be refactored to be faster/nicer
func readln(r *bufio.Reader) (string, error) {
	var (
		isPrefix = true
		err      error
		line, ln []byte
	)
	for isPrefix && err == nil {
		line, isPrefix, err = r.ReadLine()
		ln = append(ln, line...)
	}
	return string(ln), err
}

func playList(s *discordgo.Session, m *discordgo.MessageCreate, g *discordgo.Guild, qLink string) {
	url := ""
	if strings.Contains(qLink, "youtube.com") || strings.Contains(qLink, "youtu.be") {
		url = "youtu.be/"
	} else if strings.Contains(qLink, "soundcloud.com") {
		url = "api.soundcloud.com/tracks/"
	}

	ytdl := exec.Command("youtube-dl", "-i", "--get-id", qLink)
	ytdlout, err := ytdl.StdoutPipe()
	if err != nil {
		log.Println("ytdl StdoutPipe err:", err)
		return
	}

	r := bufio.NewReaderSize(ytdlout, 1024)
	if err = ytdl.Start(); err != nil {
		log.Println("ytdl Start err:", err)
		return
	}

	qCount := 0
	message, _ := s.ChannelMessageSend(m.ChannelID, "Queuing "+qLink+" Playlist"+strings.Repeat(".", (qCount%3)+1)+" Length: "+strconv.Itoa(qCount))

	// TODO: Replace with something faster/nicer
	id, err := readln(r)
	for err == nil {
		vidLink := url + id
		fmt.Println(vidLink)
		playLink(s, m, g, vidLink, true)
		qCount++
		id, err = readln(r)
		s.ChannelMessageEdit(m.ChannelID, message.ID, "Queuing "+qLink+" Playlist"+strings.Repeat(".", (qCount%3)+1)+" Length: "+strconv.Itoa(qCount))
	}
	s.ChannelMessageEdit(m.ChannelID, message.ID, "Queuing "+qLink+" Playlist! Length: "+strconv.Itoa(qCount))
}

func bytesToString(byteString []byte) string {
	return string(byteString[:])
}

func urlToString(url string) string {
	res, err := http.Get(url)
	if err != nil {
		log.Info(err)
		return "ERROR: HTTP GET BORK"
	}
	urlstring, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		log.Info(err)
		return "ERROR: HTTP TO STRING BORK"
	}
	return bytesToString(urlstring)
}

func filterYtAPIresponse(apiResponse, contentName string) (string, error) {
	if !strings.Contains(apiResponse, contentName) {
		return "", errors.New("Content not in API Response")
	}

	split := strings.SplitAfterN(apiResponse, contentName, 2)
	split = strings.SplitAfterN(split[1], "\": \"", 2)
	split = strings.SplitN(split[1], "\",", 2)
	split = strings.SplitN(split[0], "\"\n", 2)
	return split[0], nil
}

func ytIDtoAPIurl(id string) string {
	return "https://www.googleapis.com/youtube/v3/videos?id=" + id + "&key=" + YTAPIKEY
}

func isLive(ytID string) bool {
	if ytID == "" || YTAPIKEY == "" {
		return false
	}

	apiResponse, err := filterYtAPIresponse(urlToString(ytIDtoAPIurl(ytID)+"&fields=items(snippet(liveBroadcastContent))&part=snippet"), "liveBroadcastContent")
	if err != nil {
		log.Info(err)
		return false
	}
	if apiResponse == "live" {
		return true
	}
	return false
}

func cleanLink(link string) string {
	link = strings.Replace(link, "\n", "", -1)
	return strings.Replace(link, "\t", "", -1)
}

func sayGuilds(s *discordgo.Session, m *discordgo.MessageCreate) {
	guilds, _ := s.UserGuilds()
	for _, g := range guilds {
		s.ChannelMessageSend(m.ChannelID, g.ID)
	}
}

func deleteMessageIn(s *discordgo.Session, m *discordgo.Message, timeout time.Duration) {
	time.Sleep(time.Second * timeout)
	s.ChannelMessageDelete(m.ChannelID, m.ID)
}

// Handles bot operator messages, should be refactored (lmao)
func handleBotControlMessages(s *discordgo.Session, m *discordgo.MessageCreate, parts []string, g *discordgo.Guild, perm ...int) {
	perms, _ := discord.State.UserChannelPermissions(discord.State.User.ID, m.ChannelID)
	if perms&discordgo.PermissionSendMessages == 0 {
		log.Info("I don't have the permission to post")
		if m.Author.ID != OWNER {
			log.Info("...but it's the owner, so I don't care")
			return
		}
	}

	accessLevel := -1
	if perm != nil {
		accessLevel = perm[0]
	}

	var message *discordgo.Message
	var merr error
	if len(parts) == 1 {
		message, merr = s.ChannelMessageSend(m.ChannelID, "What you want?")
	} else if scontains("q", parts[1]) && len(parts) == 3 {
		playLink(s, m, g, cleanLink(parts[2]), false)
	} else if scontains("q", parts[1]) && len(parts) > 3 {
		var link string
		link, parts = parts[len(parts)-1], parts[:len(parts)-1]
		//log.Info("Testing: " + link)
		defer playLink(s, m, g, cleanLink(link), false)
		handleBotControlMessages(s, m, parts, g)
	} else if scontains("pl", parts[1]) && len(parts) == 3 {
		playList(s, m, g, cleanLink(parts[2]))
	} else if scontains("pl", parts[1]) && len(parts) > 3 {
		var link string
		link, parts = parts[len(parts)-1], parts[:len(parts)-1]
		defer playList(s, m, g, cleanLink(link))
		handleBotControlMessages(s, m, parts, g)

	} else if (scontains("syt", parts[1]) || scontains("s", parts[1])) && len(parts) > 2 {
		term := strings.Join(parts[2:], " ")
		link := searchYtForPlay(s, m, g, term)
		log.Info("YT search term: \"" + term + "\" and found " + link)
		playLink(s, m, g, cleanLink(link), false)

	} else if scontains("ssc", parts[1]) && len(parts) > 2 {
		term := strings.Join(parts[2:], " ")
		link := searchScForPlay(s, m, g, term)
		log.Info("SC search term: \"" + term + "\" and found " + link)
		playLink(s, m, g, cleanLink(link), false)

	} else if (scontains("sytm", parts[1]) || scontains("sm", parts[1])) && len(parts) > 3 {
		//var allLinks string
		num, err := strconv.Atoi(parts[2])
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Error: "+err.Error())
			log.Info(err)
			return
		}
		links := searchYtForMutliPlay(s, m, g, strings.Join(parts[3:], " "), num)
		for _, link := range links {
			playLink(s, m, g, cleanLink(link), false)
		}

	} else if scontains("lq", parts[1]) {
		listQueue(s, m, g)
	} else if scontains("live", parts[1]) && len(parts) == 3 {
		id, err := getIDFromLink(parts[2])
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Error: "+err.Error())
			return
		}
		if isLive(id) {
			message, merr = s.ChannelMessageSend(m.ChannelID, "That video is live")
		} else {
			message, merr = s.ChannelMessageSend(m.ChannelID, "Nah mate, not live")
		}
	} else if scontains("skip", parts[1]) {
		skip(g)
		message, merr = s.ChannelMessageSend(m.ChannelID, "<@"+m.Author.ID+"> skipped")
		log.Info(m.Author.ID + " skipped")
	} else if scontains("t", parts[1]) && len(parts) >= 3 {
		playTag(s, m, g, strings.Join(parts[2:], "_"))
	} else if scontains("ct", parts[1]) && len(parts) >= 4 {
		tagLink(s, m, g, strings.Join(parts[2:len(parts)-1], "_"), parts[len(parts)-1])
	} else if scontains("mt", parts[1]) && len(parts) < 3 {
		//catch case for mt
	} else if scontains("mt", parts[1]) && len(parts) >= 3 {
		var tag string
		tag, parts = parts[len(parts)-1], parts[:len(parts)-1]
		defer playTag(s, m, g, tag)
		handleBotControlMessages(s, m, parts, g)

	} else if scontains("master", parts[0]) && m.Author.ID == OWNER {
		message, merr = s.ChannelMessageSend(m.ChannelID, "Yes, Master <@"+OWNER+">.")
		parts = append(parts[:0], parts[1:]...)
		go handleBotControlMessages(s, m, parts, g, 1)
	} else if scontains("del", parts[1]) && len(parts) == 3 && accessLevel >= 0 {
		result := delDCA(parts[2])
		message, merr = s.ChannelMessageSend(m.ChannelID, result)
	} else if scontains("delTag", parts[1]) && len(parts) >= 3 && accessLevel >= 0 {
		result := delTag(strings.Join(parts[2:], "_"))
		message, merr = s.ChannelMessageSend(m.ChannelID, result)
	} else if scontains("delLink", parts[1]) && len(parts) == 3 && accessLevel >= 0 {
		result := delLink(parts[2])
		message, merr = s.ChannelMessageSend(m.ChannelID, result)
	} else if scontains("pf", parts[1]) && len(parts) == 3 && accessLevel >= 0 {
		playFile(s, m, g, parts[2])
	} else if scontains("memepost", parts[1]) && len(parts) == 2 && accessLevel >= 0 {
		gifPosting[g.ID] = !gifPosting[g.ID]
		if gifPosting[g.ID] {
			message, merr = s.ChannelMessageSend(m.ChannelID, "MEMEPOSTING ENGAGED")
		} else {
			message, merr = s.ChannelMessageSend(m.ChannelID, "The cancer has stopped...\nat least for now...")
		}
		saveServerSettings(g.ID)
	} else if scontains("memevoice", parts[1]) && len(parts) == 2 && accessLevel >= 0 {
		memeVoice[g.ID] = !memeVoice[g.ID]
		if memeVoice[g.ID] {
			message, merr = s.ChannelMessageSend(m.ChannelID, "MEMEVOICE ENGAGED, !BEES TO FUCK SHIT UP")
		} else {
			message, merr = s.ChannelMessageSend(m.ChannelID, "The cancer voice has stopped...\nat least for now...")
		}
		saveServerSettings(g.ID)
	} else if scontains("ytdlupdate", parts[1]) && len(parts) == 2 && accessLevel >= 0 {
		updateMessage := updateYTDL(s, m, g)
		s.ChannelMessageSend(m.ChannelID, updateMessage)
	} else if scontains("ytdlver", parts[1]) && len(parts) == 2 && accessLevel >= 0 {
		verMessage := verCheckYTDL(s, m, g)
		s.ChannelMessageSend(m.ChannelID, verMessage)
	} else if scontains("cache", parts[1]) && len(parts) == 2 && accessLevel >= 0 {
		caching[g.ID] = !caching[g.ID]
		if caching[g.ID] {
			message, merr = s.ChannelMessageSend(m.ChannelID, "Caching enabled")
		} else {
			message, merr = s.ChannelMessageSend(m.ChannelID, "Caching disabled")
		}
		saveServerSettings(g.ID)
	} else if scontains("memetimeout", parts[1]) && len(parts) == 3 && accessLevel >= 0 {
		newTimeout, err := time.ParseDuration(parts[2])
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Error: "+err.Error())
			log.Info(err)
			return
		}
		memeTimeout[g.ID] = newTimeout
		saveServerSettings(g.ID)

	} else if scontains("servers", parts[1]) && accessLevel == 1 {
		sayGuilds(s, m)
	} else if scontains("leave", parts[1]) && accessLevel == 1 {
		err := s.GuildLeave(parts[2])
		if err == nil {
			message, merr = s.ChannelMessageSend(m.ChannelID, "I've left "+parts[2])
		} else {
			message, merr = s.ChannelMessageSend(m.ChannelID, "Unable to leave due to error")
			log.Info(err)
		}

	} else if scontains("status", parts[1]) {
		displayBotStats(m.ChannelID)
	} else if scontains("stats", parts[1]) {
		if len(m.Mentions) >= 2 {
			displayUserStats(m.ChannelID, utilGetMentioned(s, m).ID)
		} else if len(parts) >= 3 {
			displayUserStats(m.ChannelID, parts[2])
		} else {
			displayServerStats(m.ChannelID, g.ID)
		}
	} else if scontains("aps", parts[1]) {
		message, merr = s.ChannelMessageSend(m.ChannelID, ":ok_hand: give me a sec m8")
		go calculateAirhornsPerSecond(m.ChannelID)

	} else if scontains("id", parts[1]) && len(parts) == 3 {
		id, err := getIDFromLink(parts[2])
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Error: "+err.Error())
			return
		}
		message, merr = s.ChannelMessageSend(m.ChannelID, "Link ID: `"+id+"`")
	} else if scontains("info", parts[1]) && len(parts) == 3 {
		title, id, duration, latency, err := getInfoFromLink(parts[2])
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Error: "+err.Error())
			log.Info(err)
			return
		}
		message, merr = s.ChannelMessageSend(m.ChannelID, "Name: `"+title+"`\nID: `"+id+"`\nDuration: `"+timeFormat(duration)+"`\nLatency:`"+latency+"`")
	} else if scontains("help", parts[1]) {
		s.ChannelMessageSend(m.ChannelID, "`@AirGoat cmd`")
		s.ChannelMessageSend(m.ChannelID, "Command list: `q` - Queues a YouTube or SoundCloud link\n`pl` - Queues a YouTube or SoundCloud playlist\n`t` - Queues a tag\n`ct` - Creates a tag\n`mt` - Queues multiple tags\n`skip` - Skips current song\n`help` - This")
		//s.ChannelMessageSend(m.ChannelID, "`master @AirGoat cmd`")
		//s.ChannelMessageSend(m.ChannelID, "Command list: `del` `delTag` `delLink` `pf` `gifpost` `cache` `servers` `leave`")
	} else {
		message, merr = s.ChannelMessageSend(m.ChannelID, "Stop.")
	}

	if merr != nil {
		log.Info("Message send error: " + merr.Error())
	} else if message != nil {
		go deleteMessageIn(s, message, 5)
	}
}

func gifPost(s *discordgo.Session, m *discordgo.MessageCreate, g *discordgo.Guild) {
	if time.Now().Before(lastMeme[g.ID].Add(memeTimeout[g.ID])) {
		log.Info("too soon")
		return
	}

	f, err := os.Open("memes.csv")
	if err != nil {
		log.Info(err)
		return
	}
	defer f.Close()

	r := csv.NewReader(f)

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Info(err)
			return
		}
		if len(record) < 2 {
			continue
		}
		if len(m.Content) >= 1 && strings.Contains(strings.ToLower(m.Content), record[0]) {
			meme := strings.Replace(record[1], "\\n", "\n", -1)
			s.ChannelMessageSend(m.ChannelID, meme)
		}
	}

	lastMeme[g.ID] = time.Now()
}

func onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == "129329923595829248" {
		s.ChannelMessageDelete(m.ChannelID, m.ID)
		return
	}

	channel, _ := discord.State.Channel(m.ChannelID)
	if channel == nil {
		log.WithFields(log.Fields{
			"channel": m.ChannelID,
			"message": m.ID,
		}).Warning("Failed to grab channel")
		return
	}

	guild, _ := discord.State.Guild(channel.GuildID)
	if guild == nil {
		log.WithFields(log.Fields{
			"guild":   channel.GuildID,
			"channel": channel,
			"message": m.ID,
		}).Warning("Failed to grab guild")
		return
	}

	if gifPosting[guild.ID] && m.Author.ID != s.State.User.ID {
		go gifPost(s, m, guild)
	}

	if len(m.Content) <= 0 || (m.Content[0] != '!' && len(m.Mentions) < 1) {
		return
	}

	msg := strings.Replace(m.ContentWithMentionsReplaced(), s.State.Ready.User.Username, "username", 1)
	parts := strings.Split(msg, " ")

	// If this is a mention, it should come from the owner (otherwise we don't care)
	if len(m.Mentions) > 0 && len(parts) > 0 {
		mentioned := false
		for _, mention := range m.Mentions {
			mentioned = (mention.ID == s.State.Ready.User.ID)
			if mentioned {
				break
			}
		}

		if mentioned {
			handleBotControlMessages(s, m, parts, guild)
		}
		return
	}

	// Find the collection for the command we got
	for _, coll := range COLLECTIONS {
		if scontains(parts[0], coll.Commands...) && memeVoice[guild.ID] == true {

			// If they passed a specific sound effect, find and select that (otherwise play nothing)
			var sound *Sound
			if len(parts) > 1 {
				for _, s := range coll.Sounds {
					if parts[1] == s.Name {
						sound = s
					}
				}

				if sound == nil {
					return
				}
			}

			go enqueuePlay(m.Author, guild, coll, sound)
			return
		}
	}
}

//Pray to the gods this works how I want it to, hell, thinking about it, it really shouldn't work
func makeQueueList(guildID string) {
	f, err := os.Create("squeues/" + guildID + ".csv")
	if err != nil {
		log.Println("file creation err:", err)
		return
	}
	f.Close()
}

func advanceQueueList(guildID string) {
	csvFile := "squeues/" + guildID + ".csv"
	f, err := os.Open(csvFile)
	if err != nil {
		if err.Error() == "open "+csvFile+": The system cannot find the file specified." || err.Error() == "open "+csvFile+": no such file or directory" {
			f.Close()
			log.Info("making queue file for ", guildID)
			makeQueueList(guildID)
		} else {
			log.Info("queue load err: ", err)
		}
		return
	}
	defer f.Close()
	r := csv.NewReader(f)
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		recordlen := len(record)
		log.Info(recordlen, " _ ", record)
		if recordlen >= 1 {
			log.Info("First queue for " + guildID + " is " + record[0])
			for i := 1; i < recordlen; i++ {
				record[i-1] = record[i]
			}
			log.Info("For guildID " + guildID + " first queue is now" + record[0])
		}

	}

}

func addToQueueList(guildID string, link string) bool {
	file := "squeues/" + guildID + ".csv"
	f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0777)
	if err.Error() == "open "+file+": The system cannot find the file specified." || err.Error() == "open "+file+": no such file or directory" {
		f.Close()
		log.Info("Could not load, making file, err: ", err)
		makeQueueList(guildID)
		f, err = os.OpenFile("squeues/"+guildID+".csv", os.O_APPEND|os.O_WRONLY, 0777)
		if err != nil {
			log.Info("Cannot open file still, err: ", err)
		}
	} else if err != nil {
		log.Info("Queue file err: ", err)
		return false
	}
	defer f.Close()
	if _, err = f.WriteString("\"" + link + "\","); err != nil {
		log.Info(err)
		return false
	}
	log.Info(err)
	return true
}

func saveServerSettings(guildID string) {
	err := os.Truncate("sconfigs/"+guildID+".csv", 0)
	if err != nil {
		log.Info("error emptying file: ", err)
		return
	}

	f, err := os.OpenFile("sconfigs/"+guildID+".csv", os.O_WRONLY|os.O_APPEND, 0777)
	if err != nil {
		log.Info("server settings save err: ", err)
		return
	}
	n, err := f.WriteString(toCSV(strconv.FormatBool(gifPosting[guildID]), strconv.FormatBool(caching[guildID]), memeTimeout[guildID].String(), strconv.FormatBool(memeVoice[guildID])))
	log.Info(n, err)
	f.Sync()
	f.Close()
}

func toCSV(args ...string) string {
	csValue := []string{"\"", "", "\""}
	for i, arg := range args {
		csValue[1] = arg
		args[i] = strings.Join(csValue, "")
	}
	return strings.Join(args, ",")
}

func initServerSettings(guildID string) {
	gifPosting[guildID] = false
	caching[guildID] = false
	memeTimeout[guildID], _ = time.ParseDuration("0s")
	memeVoice[guildID] = true

	f, err := os.Create("sconfigs/" + guildID + ".csv")
	if err != nil {
		log.Println("file creation err:", err)
		return
	}
	f.Close()
	/*
		f, err = os.Create("squeues/" + guildID + ".csv")
		if err != nil {
			log.Println("file creation err:", err)
			return
		}
		f.Close()

	*/
	saveServerSettings(guildID)
}

func loadServerSettings(guildID string) {
	csvFile := "sconfigs/" + guildID + ".csv"
	f, err := os.Open(csvFile)
	if err != nil {
		if err.Error() == "open "+csvFile+": The system cannot find the file specified." || err.Error() == "open "+csvFile+": no such file or directory" {
			f.Close()
			log.Info("creating server settings")
			initServerSettings(guildID)
		} else {
			log.Info("server setting load err: ", err)
		}
		return
	}
	defer f.Close()

	r := csv.NewReader(f)

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Info(err)
			return
		}
		if len(record) < 4 {
			log.Info("invalid settings length")
			return
		}
		gifPosting[guildID], err = strconv.ParseBool(record[0])
		if err != nil {
			gifPosting[guildID] = false
			log.Info(err)
		}
		caching[guildID], err = strconv.ParseBool(record[1])
		if err != nil {
			caching[guildID] = false
			log.Info(err)
		}
		memeTimeout[guildID], err = time.ParseDuration(record[2])
		if err != nil {
			memeTimeout[guildID], _ = time.ParseDuration("0s")
			log.Info(err)
		}
		memeVoice[guildID], err = strconv.ParseBool(record[3])
		if err != nil {
			memeVoice[guildID] = true
			log.Info(err)
		}
	}
}

/*func getCurrentVoiceChannel(user *discordgo.User, guild *discordgo.Guild) *discordgo.Channel {
	for _, vs := range guild.VoiceStates {
		if vs.UserID == user.ID {
			channel, _ := discord.State.Channel(vs.ChannelID)
			return channel
		}
	}
	return nil
}*/

func searchYtForPlay(s *discordgo.Session, m *discordgo.MessageCreate, g *discordgo.Guild, thingsToFind string) string {
	log.Info("searching using: " + thingsToFind)
	cmd := exec.Command("youtube-dl", "ytsearch1:"+thingsToFind, "--get-id")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Info(err)
		return "Not Found"
	}
	return "http://youtu.be/" + strings.Split(out.String(), "\n")[0]
}

func searchScForPlay(s *discordgo.Session, m *discordgo.MessageCreate, g *discordgo.Guild, thingsToFind string) string {
	cmd := exec.Command("youtube-dl", "scsearch1:"+thingsToFind, "--get-url")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Info(err)
		return "Not Found"
	}
	//SCinfo := getScInfoFromId(info[0])
	return strings.Split(out.String(), "\n")[0]
}

func searchYtForMutliPlay(s *discordgo.Session, m *discordgo.MessageCreate, g *discordgo.Guild, thingsToFind string, numberOfthings int) []string {
	cmd := exec.Command("youtube-dl", "ytsearch"+strconv.Itoa(numberOfthings)+":"+thingsToFind, "--get-id")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Info(err)
		return nil
	}
	IDs := strings.Split(out.String(), "\n")
	var links []string
	for _, ID := range IDs[:len(IDs)-1] {
		links = append(links, "http://youtu.be/"+ID)
	}
	return links
}

func updateYTDL(s *discordgo.Session, m *discordgo.MessageCreate, g *discordgo.Guild) string {
	cmd := exec.Command("youtube-dl", "-U")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Info(err)
		return "YTDL cmd Error"
	}
	return strings.Replace(out.String(), "\n", " ", -1)
}

func verCheckYTDL(s *discordgo.Session, m *discordgo.MessageCreate, g *discordgo.Guild) string {
	cmd := exec.Command("youtube-dl", "--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Info(err)
		return "YTDL cmd Error"
	}
	return strings.Replace(out.String(), "\n", " ", -1)
}

func main() {
	var (
		Token      = flag.String("t", "", "Discord Authentication Token")
		Redis      = flag.String("r", "", "Redis Connection String")
		Shard      = flag.String("s", "", "Shard ID")
		ShardCount = flag.String("c", "", "Number of shards")
		Owner      = flag.String("o", "", "Owner ID")
		YtAPIKey   = flag.String("y", "", "Youtube API Key")
		err        error
	)
	flag.Parse()

	if *Owner != "" {
		OWNER = *Owner
	}

	if *YtAPIKey != "" {
		YTAPIKEY = *YtAPIKey
	} else {
		log.Info("WARNING! You have not provided a YouTube API Key .. @AirGoat live function will not work correctly .. Caching is dangerous as Live YouTube videos are not checked")
	}

	// Preload all the sounds
	log.Info("Preloading sounds...")
	for _, coll := range COLLECTIONS {
		coll.Load()
	}

	// If we got passed a redis server, try to connect
	if *Redis != "" {
		log.Info("Connecting to redis...")
		rcli = redis.NewClient(&redis.Options{Addr: *Redis, DB: 0})
		_, err = rcli.Ping().Result()

		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Fatal("Failed to connect to redis")
			return
		}
	}

	// Create a discord session
	log.Info("Starting discord session...")
	discord, err = discordgo.New(*Token)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("Failed to create discord session")
		return
	}

	// Set sharding info
	discord.ShardID, _ = strconv.Atoi(*Shard)
	discord.ShardCount, _ = strconv.Atoi(*ShardCount)

	if discord.ShardCount <= 0 {
		discord.ShardCount = 1
	}

	guilds, _ := discord.UserGuilds()
	for _, g := range guilds {
		loadServerSettings(g.ID)
	}

	discord.AddHandler(onReady)
	discord.AddHandler(onGuildCreate)
	discord.AddHandler(onMessageCreate)

	err = discord.Open()
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Fatal("Failed to create discord websocket connection")
		return
	}

	// We're running!
	log.Info("AIRGOAT is ready to BEES.")

	// Wait for a signal to quit
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	<-c
}
