package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"strconv"
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
	// Check if first word is man!
	if words[0] != "man!" {
		return
	}
	fmt.Println(m.Author.Username + "#" + m.Author.Discriminator + ": " + m.Content)
	if len(words) == 2 {
		manpage(s, m, "0", words[1])
	}
	if len(words) == 3 {
		manpage(s, m, words[1], words[2])
	}
}

func getManPage(section string, command string) (status int, body string, err error) {
	resp, err := http.Get(k.String("man.server") + section + "/" + command)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return 0, "", err
	}
	status = resp.StatusCode
	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	return status, string(bodyBytes), nil
}

func manpage(s *discordgo.Session, m *discordgo.MessageCreate, section string, command string) {
	body := ""
	err := error(nil)
	status := 0
	if section == "0" {
		for i := 1; i < 10; i++ {
			status, body, _ = getManPage(strconv.Itoa(i), command)
			if status == 200 {
				section = strconv.Itoa(i)
				break
			}
		}
		if status != 200 {
			s.ChannelMessageSend(m.ChannelID, "No manpage found for "+command)
			return
		}

	} else {

		status, body, err = getManPage(section, command)

		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Internal server error")
			return
		}

		if status != 200 {
			switch status {
			case 404:
				s.ChannelMessageSend(m.ChannelID, "No manpage found for "+section+" "+command)
				return
			case 500:
				fmt.Println("Internal server error" + string(body))
				s.ChannelMessageSend(m.ChannelID, "Internal server error")
				return
			default:
				fmt.Println("Unknown error code: " + string(status) + " " + string(body))
				s.ChannelMessageSend(m.ChannelID, "Unknown error")
				return
			}
		}
	}
	//split it by zero width space
	sections := strings.SplitAfter(string(body), "\u200b")

	// Make an embed with each word as its own embed and send to discord
	embeds := []*discordgo.MessageEmbed{}
	for _, section := range sections {
		// split on the first newline
		split := strings.SplitN(section, "\n", 2)
		title := split[0]
		if len(split) == 1 {
			break
		}
		description := split[1]
		if len(description) > 4096 {
			for len(description) > 4096 {
				embeds = append(embeds, &discordgo.MessageEmbed{
					Title:       title,
					Description: description[:4096],
				})
				description = description[4096:]
			}
		}
		embeds = append(embeds, &discordgo.MessageEmbed{
			Title:       title,
			Description: description,
		})
	}
	// Make thread
	thread, err := s.MessageThreadStart(m.ChannelID, m.ID, "man "+section+" "+command, 60)
	if err != nil {
		fmt.Println(err)
		return
	}

	for _, embed := range embeds {
		_, err := s.ChannelMessageSendEmbed(thread.ID, embed)
		if err != nil {
			fmt.Println(err)
		}
	}

}
