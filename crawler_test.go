package crawler

import (
  "fmt"
  "sync"
  "testing"
)

var wg sync.WaitGroup

var testRule = []byte(`id: "jd"
version: 1
name: "jd"
alias: "京东"
group: "default"
priority: 100

patterns:
  - "jd.com"

timeout: "10s"

fields:
  - name: price
    eval: "document.querySelector('.J-p-100000700300').textContent"
    export: true

loop:
  name: "comment"
  alias: "最新12页评论"
  export_cycle: 5
  prepare:
    eval: "{document.documentElement.scrollBy(0, 1000);Array.prototype.slice.call(document.querySelector('#detail > div > ul').children).filter(function (e) {return e.textContent.indexOf('商品评价') !== -1;})[0].click();true;}"
    wait: "5s"
  eval: "JSON.stringify(Array.prototype.slice.call(document.querySelectorAll('.comment-con')).map(e=>e.textContent))"
  next: "document.querySelector('.ui-pager-next').click();cdp_loop_count<=11"
  wait: "2s"
`)

type H struct {
  name string
}

func (h *H) OnFields(p *Page, data map[string]string) {
  fmt.Println("======OnFields:", data)
}

func (h *H) OnLoop(p *Page, loopCount int, data []string) {
  fmt.Println("======OnLoop:", loopCount, data)
}

func (h *H) OnComplete(p *Page) {
  fmt.Println("======OnComplete")
  wg.Done()
}

func TestCrawler(t *testing.T) {
  e := Rules.FromBytes([][]byte{testRule})
  if e != nil {
    t.Fatal(e)
  }
  e = LaunchChrome("")
  if e != nil {
    t.Fatal(e)
  }
  wg.Add(1)
  h := &H{"JingDong"}
  p := NewPage("01", "https://item.jd.com/100000700300.html", "")
  e = p.Crawl(h)
  if e != nil {
    panic(e)
  }
  wg.Wait()
}
