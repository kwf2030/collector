package collector

import (
  "fmt"
  "runtime"
  "sync"
  "testing"

  "github.com/kwf2030/cdp"
)

var wg1 sync.WaitGroup

var rule1 = []byte(`id: "jd"
version: 1
name: "jd"
alias: "京东"
group: "default"
priority: 100

patterns:
  - "jd.com"

timeout: "10s"

fields:
  - name: title
    eval: "document.querySelector('.sku-name').textContent.trim()"
    export: true

  - name: price
    eval: "document.querySelector('.J-p-100000700300').textContent.trim()"
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

type Product struct{}

func (*Product) OnFields(p *Page, data map[string]string) {
  fmt.Println("==========OnFields:")
  fmt.Println(data)
}

func (*Product) OnLoop(p *Page, loopCount int, data []string) bool {
  fmt.Println("==========OnLoop:", loopCount)
  fmt.Println(data)
  return true
}

func (*Product) OnComplete(p *Page) {
  fmt.Println("==========OnComplete")
  wg1.Done()
}

func TestProduct(t *testing.T) {
  bin := ""
  switch runtime.GOOS {
  case "windows":
    bin = "C:/App/Chromium/chrome.exe"
  case "linux":
    bin = "/usr/bin/google-chrome-stable"
  }
  chrome, e := cdp.Launch(bin)
  if e != nil {
    t.Fatal(e)
  }

  rg := NewRuleGroup("default")
  e = rg.AppendBytes(rule1)
  if e != nil {
    t.Fatal(e)
  }

  p := NewPage("01", "https://item.jd.com/100000700300.html", "default")
  e = p.Collect(chrome, rg, &Product{})
  if e != nil {
    t.Fatal(e)
  }

  wg1.Add(1)
  wg1.Wait()
  _ = chrome.Exit()
}
