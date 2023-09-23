package claimer

import (
	"os"
	"time"

	"github.com/Kqzz/MCsniperGO/mc"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpproxy"

	"github.com/Kqzz/MCsniperGO/log"
)

var workerCount = 100

type Claim struct {
	Username  string
	Running   bool
	DropRange mc.DropRange
	Accounts  []*mc.MCaccount
	Proxies   []string
}

func (c *Claim) Start() {
	c.Running = true
	go c.runClaim()
}

func (c *Claim) Stop() {
	c.Running = false
}

type ClaimAttempt struct {
	Name    string
	Bearer  string
	AccType mc.AccType
	AccNum  int
	Proxy   string
}

func requestGenerator(
	workChan chan ClaimAttempt,
	killChan chan bool,
	bearers []string,
	name string,
	accType mc.AccType,
	endTime time.Time,
	proxies []string,
	delay int,
) {
	noEnd := endTime.IsZero()
	if len(bearers) == 0 {
		return
	}

	sleepTime := delay

	if delay == -1 {
		sleepTime = 15000 / len(bearers)
		if accType == mc.Ms {
			sleepTime = 10000 / len(bearers)
		}
	}
	loopCount := 2
	if accType == mc.Ms {
		loopCount = 3
	}
	i := 0
	prox := 0
	for noEnd || time.Now().Before(endTime) {
		for y := 0; y < loopCount; y++ { // run n times / bearer
			if i >= len(bearers) {
				i = 0
			}

			if prox >= len(proxies) {
				prox = 0
			}

			workChan <- ClaimAttempt{
				Name:    name,
				Bearer:  bearers[i],
				AccType: accType,
				Proxy:   proxies[prox],
				AccNum:  i + 1,
			}
			time.Sleep(time.Millisecond * time.Duration(sleepTime))
			prox++
		}
		i++
	}

}

func claimName(claim ClaimAttempt, client *fasthttp.Client) {
	acc := mc.MCaccount{
		Bearer: claim.Bearer,
		Type:   claim.AccType,
	}

	status := 0
	var err error = nil

	if claim.Proxy != "" {
		client.Dial = fasthttpproxy.FasthttpSocksDialer(claim.Proxy)
	}

	before := time.Now()
	if claim.AccType == mc.Ms {
		status, err = acc.ChangeUsername(claim.Name, client)
	} else {
		status, err = acc.CreateProfile(claim.Name, client)
	}
	after := time.Now()

	if err != nil {
		log.Log("err", "%v #%d", err, claim.AccNum)
		return
	}

	log.Log("info", "%v %vms [%v] %v %v #%d", after.Format("15:04:05.999"), after.Sub(before).Milliseconds(), claim.Name, status, acc.Type, claim.AccNum)
	if status == 200 {
		log.Log("success", "Claimed %v on %v acc, %v", claim.Name, acc.Type, acc.Bearer[len(acc.Bearer)/2:])
		log.Log("success", "Join https://discord.gg/2BZseKW for more!")
	}
	
	if status == 401 {
		log.Log("err", "restart: %v", "Lost authorization")
		os.Exit(0)
	}
}

func worker(claimChan chan ClaimAttempt, killChan chan bool) {
	client := &fasthttp.Client{
		Dial: fasthttp.Dial,
	}
	for {
		select {
		case claim := <-claimChan:
			claimName(claim, client)
		case <-killChan:
			return
		}
	}
}

func (s *Claim) runClaim() {
	workChan := make(chan ClaimAttempt)
	killChan := make(chan bool)
	s.Running = true

	go func() {
		for {
			if !s.Running {
				log.Log("info", "Stopped claim of %v", s.Username)
				close(killChan)
				return
			}
			time.Sleep(time.Second * 5)
		}
	}()

	gcs := []string{}
	mss := []string{}

	for _, acc := range s.Accounts {
		if acc.Type == mc.Ms {
			mss = append(mss, acc.Bearer)
		} else {
			gcs = append(gcs, acc.Bearer)
		}
	}

	for i := 0; i < workerCount; i++ {
		go worker(workChan, killChan)
	}

	log.Log("info", "using %v accounts", len(s.Accounts))
	log.Log("info", "using %v proxies", len(s.Proxies))

	if len(s.Proxies) == 0 {
		s.Proxies = []string{""}
	}

	time.Sleep(time.Until(s.DropRange.Start))

	go requestGenerator(workChan, killChan, gcs, s.Username, mc.MsPr, s.DropRange.End, s.Proxies, -1)
	go requestGenerator(workChan, killChan, mss, s.Username, mc.Ms, s.DropRange.End, s.Proxies, -1)

	if s.DropRange.End.IsZero() {
		select {}
	}

	for time.Now().Before(s.DropRange.End) {
		time.Sleep(10 * time.Second)
	}
	s.Running = false
	_, ok := (<-killChan)
	if ok {
		close(killChan)
	}

}
