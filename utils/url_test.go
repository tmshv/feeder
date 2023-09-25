package utils

import "testing"

func TestNoUtmUrls(t *testing.T) {
    urls := []string{
        "https://web-standards.ru/podcast/387/",
        "https://machinelearning.apple.com/research/hyperdiffusion",
        "https://lea.verou.me/blog/2023/state-of-html-2023/",
        "https://vercel.com/changelog/vercel-toolbar-now-available-to-use-collaboration-features-in-production",
    }

    for _, url := range urls {
        result := DropUtmMarkers(url)
        if result != url {
            t.Errorf("Drop UTM brokes a url %s", url)
        }
    }
}

func TestUrlsWithUtm(t *testing.T) {
    withUrls := []string{
        "https://journal.tinkoff.ru/diary-analitik-dannyh-tbilisi-530k/?utm_source=rss",
    }
    withoutUrls := []string{
        "https://journal.tinkoff.ru/diary-analitik-dannyh-tbilisi-530k/",
    }

    for i, url := range withUrls {
        result := DropUtmMarkers(url)
        if result != withoutUrls[i] {
            t.Errorf("Droping UTM is incorrect for url %s", url)
        }
    }
}
