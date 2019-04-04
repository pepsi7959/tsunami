package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Cmd struct {
	Func func(string) error
}

type Shell struct {
	CmdList      map[string]Cmd
	Done         *bool
	enableReport *bool

	//show menu
	enableMenu bool
}

func (s *Shell) Init() {
	s.CmdList = make(map[string]Cmd)
	s.enableMenu = false
}

func (s *Shell) AddCmd(cn string, f func(string) error) {
	s.CmdList[cn] = Cmd{Func: f}
}

func (s *Shell) Run() {
	var oldEnableReport bool

	for *s.Done == false {
		r := bufio.NewReader(os.Stdin)
		txt, _ := r.ReadString('\n')
		if txt == "\n" {
			if s.enableMenu == false {
				s.CmdList["help"].Func("")
				s.enableMenu = true
				oldEnableReport = *s.enableReport
				*s.enableReport = false

			} else {
				s.enableMenu = false
				*s.enableReport = oldEnableReport
				fmt.Println("\033[H\033[2J")
			}
		} else {
			str := strings.TrimSuffix(txt, "\n")
			clist := strings.SplitN(str, " ", 2)
			cmd, f := s.CmdList[clist[0]]

			var param string

			if len(clist) > 1 {
				param = clist[1]
			}
			if f == true {
				err := cmd.Func(param)

				if err != nil {
					fmt.Println("Err: ", err)
				}

				s.enableMenu = false
			}
		}
	}
}
