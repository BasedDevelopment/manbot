package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const (
	configPath = "config.toml"
)

var (
	k      = koanf.New(".")
	parser = toml.Parser()
)

func main() {
	if err := k.Load(file.Provider(configPath), toml.Parser()); err != nil {
		fmt.Println(err)
		return
	}
	if k.String("discord.token") == "" {
		fmt.Println("No discord token found")
		return
	}
	if k.String("man.server") == "" {
		fmt.Println("No manpage server url found")
		return
	}
	dg, err := discordgo.New("Bot " + k.String("discord.token"))
	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}
	dg.Identify.Intents |= discordgo.IntentsGuildMessages
	dg.AddHandler(messageCreate)

	err = dg.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}
	fmt.Println("Bot is now running.  Press CTRL-C to exit.")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
	dg.Close()
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}
	// Split message by spaces
	words := strings.Split(m.Content, " ")
	// Check if first word is manpage
	if len(words) != 3 || words[0] != "man!" {
		return
	}
	manpage(s, m, words[1], words[2])

}

func manpage(s *discordgo.Session, m *discordgo.MessageCreate, section string, command string) {
	resp, _ := http.Get(k.String("man.server") + section + "/" + command)
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	// check err code
	if resp.StatusCode != 200 {
		s.ChannelMessageSend(m.ChannelID, "Error: "+string(body))
		return
	}
	//split it by new ZWSC
	words := strings.SplitAfter(string(body), "\n\n")

	// Make an embed with each word as its own embed and send to discord
	embeds := []*discordgo.MessageEmbed{}
	for _, word := range words {
		// split on the first newline
		split := strings.SplitN(word, "\n", 2)
		title := split[0]
		description := split[1]
		embeds = append(embeds, &discordgo.MessageEmbed{
			Title:       title,
			Description: description,
		})
	}
	// Make thread
	thread, err := s.MessageThreadStart(m.ChannelID, m.ID, "man "+section+" "+command, 60)
	if err != nil {
		fmt.Println(err)
	}

	for _, embed := range embeds {
		s.ChannelMessageSendEmbed(thread.ID, embed)
	}

}
