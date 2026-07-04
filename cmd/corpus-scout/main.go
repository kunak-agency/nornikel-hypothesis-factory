// corpus-scout — авторазведчик новых профильных источников для базы знаний.
//
// Ищет книги по открытому каталогу Geokniga (тот же источник, из которого
// вручную собран текущий книжный корпус), отфильтровывает уже загруженное и
// нерелевантное, и (в режиме -download) отправляет новые PDF в собственный
// ingestion-пайплайн через POST /v1/documents — то есть новые книги проходят
// тот же Docling+OCR+BGE-M3 путь, что и остальной корпус.
//
// Сторонние нейросети не используются: поиск — каталожный, фильтр
// релевантности — детерминированный по ключевым словам, опционально
// (-llm-filter) дорабатывается через тот же Yandex GPT, что и остальной
// пайплайн.
//
// По умолчанию — dry-run (только отчёт о кандидатах, ничего не скачивает).
// Продакшен-запуск — кроном хоста, например раз в сутки:
//
//	0 3 * * * cd /opt/hypothesis-factory && ./corpus-scout -download -max 2 >> /var/log/corpus-scout.log 2>&1
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"hypothesis-factory/externalApi"
)

const (
	geoknigaBase = "https://www.geokniga.org"
	maxFileBytes = 200 << 20 // защита от гигантских файлов
)

// defaultQueries — профильные направления Норникеля: не только флотация
// Cu-Ni, но и вся деятельность (благородные металлы, гидрометаллургия,
// хвостовое хозяйство).
var defaultQueries = []string{
	"обогащение медно-никелевых руд",
	"флотация сульфидных руд",
	"гидрометаллургия никеля",
	"металлургия платиновых металлов",
	"извлечение золота и серебра",
	"хвостохранилище обогатительной фабрики",
}

// relevanceKeywords — детерминированный фильтр релевантности: название
// кандидата должно содержать хотя бы одно из этих слов (в нижнем регистре).
var relevanceKeywords = []string{
	"обогащен", "флотац", "измельчен", "дроблен", "гидроциклон", "классификац",
	"руд", "никел", "медь", "медно", "золот", "серебр", "платин", "палладий",
	"кобальт", "металлург", "хвост", "сгущен", "концентрат",
}

var (
	bookLinkRe = regexp.MustCompile(`href="(/books/\d+)"[^>]*>([^<]{10,200})</a>`)
	fileLinkRe = regexp.MustCompile(`href="(https?://www\.geokniga\.org/bookfiles/[^"]+\.pdf)"`)
)

type candidate struct {
	Title   string
	PageURL string
	FileURL string
}

func main() {
	apiBase := flag.String("api", "http://localhost:8080", "базовый URL API hypothesis-factory")
	maxIngest := flag.Int("max", 2, "максимум новых книг за прогон (бюджет ingestion)")
	download := flag.Bool("download", false, "скачивать и загружать найденное (по умолчанию dry-run: только отчёт)")
	llmFilter := flag.Bool("llm-filter", false, "дополнительный фильтр релевантности через Yandex GPT (нужны YANDEX_API_KEY/YANDEX_FOLDER_ID)")
	queriesFlag := flag.String("queries", "", "поисковые запросы через ';' (по умолчанию — профильные направления Норникеля)")
	flag.Parse()

	queries := defaultQueries
	if *queriesFlag != "" {
		queries = strings.Split(*queriesFlag, ";")
	}

	ctx := context.Background()
	httpc := &http.Client{Timeout: 120 * time.Second}

	existing, err := existingTitles(ctx, httpc, *apiBase)
	if err != nil {
		log.Fatalf("получение списка документов из API: %v", err)
	}
	log.Printf("в базе знаний %d документов", len(existing))

	var llm externalApi.LLMClient
	if *llmFilter {
		key, folder := os.Getenv("YANDEX_API_KEY"), os.Getenv("YANDEX_FOLDER_ID")
		if key == "" || folder == "" {
			log.Fatal("-llm-filter требует YANDEX_API_KEY и YANDEX_FOLDER_ID")
		}
		llm = externalApi.NewYandexClient(key, folder, "gpt://"+folder+"/yandexgpt-lite/latest")
	}

	seen := map[string]bool{}
	var fresh []candidate
	for _, q := range queries {
		cands, err := searchGeokniga(ctx, httpc, strings.TrimSpace(q))
		if err != nil {
			log.Printf("поиск %q: %v (пропускаю запрос)", q, err)
			continue
		}
		for _, c := range cands {
			norm := normalizeTitle(c.Title)
			if seen[norm] {
				continue
			}
			seen[norm] = true
			if isKnown(norm, existing) {
				continue
			}
			if !keywordRelevant(norm) {
				continue
			}
			if llm != nil && !llmRelevant(ctx, llm, c.Title) {
				log.Printf("LLM-фильтр отклонил: %s", c.Title)
				continue
			}
			fresh = append(fresh, c)
		}
	}

	log.Printf("новых релевантных кандидатов: %d", len(fresh))
	for i, c := range fresh {
		log.Printf("  %d. %s — %s", i+1, c.Title, geoknigaBase+c.PageURL)
	}

	if !*download {
		log.Printf("dry-run: ничего не скачано (для загрузки добавьте -download)")
		return
	}

	ingested := 0
	for _, c := range fresh {
		if ingested >= *maxIngest {
			log.Printf("достигнут лимит -max=%d, остальные кандидаты отложены до следующего прогона", *maxIngest)
			break
		}
		if err := ingestCandidate(ctx, httpc, *apiBase, &c); err != nil {
			log.Printf("загрузка %q: %v", c.Title, err)
			continue
		}
		log.Printf("загружено: %s", c.Title)
		ingested++
	}
	log.Printf("итог прогона: загружено %d из %d кандидатов", ingested, len(fresh))
}

// existingTitles — нормализованные заголовки уже загруженных документов.
func existingTitles(ctx context.Context, httpc *http.Client, apiBase string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/v1/documents", nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload struct {
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(payload.Items))
	for _, it := range payload.Items {
		out = append(out, normalizeTitle(it.Title))
	}
	return out, nil
}

// searchGeokniga — поиск по каталогу; HTML-парсинг регэкспом сознательно
// минимальный (это разведчик, а не парсер общего назначения) — при смене
// вёрстки каталога прогон честно вернёт 0 кандидатов, а не мусор.
func searchGeokniga(ctx context.Context, httpc *http.Client, query string) ([]candidate, error) {
	u := geoknigaBase + "/books?title=" + url.QueryEscape(query)
	page, err := fetchText(ctx, httpc, u)
	if err != nil {
		return nil, err
	}
	var out []candidate
	for _, m := range bookLinkRe.FindAllStringSubmatch(page, 20) {
		out = append(out, candidate{PageURL: m[1], Title: html.UnescapeString(strings.TrimSpace(m[2]))})
	}
	return out, nil
}

func fetchText(ctx context.Context, httpc *http.Client, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: status %d", u, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return string(b), err
}

func normalizeTitle(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'а' && r <= 'я', r == 'ё', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// isKnown — дедуп против базы по word-overlap заголовков: точного равенства
// мало (у нас заголовки с авторами/годами, в каталоге — «голые»).
func isKnown(normTitle string, existing []string) bool {
	words := strings.Fields(normTitle)
	if len(words) == 0 {
		return true
	}
	for _, ex := range existing {
		exSet := map[string]bool{}
		for _, w := range strings.Fields(ex) {
			exSet[w] = true
		}
		hit := 0
		for _, w := range words {
			if exSet[w] {
				hit++
			}
		}
		if float64(hit)/float64(len(words)) >= 0.6 {
			return true
		}
	}
	return false
}

func keywordRelevant(normTitle string) bool {
	for _, kw := range relevanceKeywords {
		if strings.Contains(normTitle, kw) {
			return true
		}
	}
	return false
}

func llmRelevant(ctx context.Context, llm externalApi.LLMClient, title string) bool {
	resp, err := llm.Complete(ctx, externalApi.CompleteRequest{
		Messages: []externalApi.Message{
			{Role: "system", Content: "Ты фильтруешь книги для базы знаний по обогащению и металлургии руд цветных и благородных металлов (Норникель). Ответь строго одним словом: да или нет."},
			{Role: "user", Content: "Релевантна ли книга: " + title},
		},
		Temperature: 0,
		MaxTokens:   5,
	})
	if err != nil {
		// При недоступном LLM не блокируем прогон — детерминированный
		// keyword-фильтр уже отработал.
		log.Printf("llm-фильтр недоступен (%v), пропускаю кандидата по keyword-фильтру", err)
		return true
	}
	return strings.Contains(strings.ToLower(resp.Text), "да")
}

// ingestCandidate: страница книги → прямая ссылка на PDF → скачивание →
// POST /v1/documents (тот же ingestion-путь, что и у ручной загрузки).
func ingestCandidate(ctx context.Context, httpc *http.Client, apiBase string, c *candidate) error {
	page, err := fetchText(ctx, httpc, geoknigaBase+c.PageURL)
	if err != nil {
		return fmt.Errorf("страница книги: %w", err)
	}
	m := fileLinkRe.FindStringSubmatch(page)
	if m == nil {
		return fmt.Errorf("на странице нет прямой PDF-ссылки (DJVU/архивы разведчик не конвертирует)")
	}
	c.FileURL = m[1]

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.FileURL, nil)
	if err != nil {
		return err
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return fmt.Errorf("скачивание: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("скачивание: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFileBytes))
	if err != nil {
		return fmt.Errorf("чтение файла: %w", err)
	}
	if len(data) < 10<<10 || !bytes.HasPrefix(data, []byte("%PDF")) {
		return fmt.Errorf("скачанное не похоже на валидный PDF (%d байт)", len(data))
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", "corpus-scout.pdf")
	if err != nil {
		return err
	}
	if _, err := part.Write(data); err != nil {
		return err
	}
	_ = w.WriteField("title", c.Title)
	_ = w.WriteField("sourceType", "book")
	_ = w.WriteField("language", "ru")
	if err := w.Close(); err != nil {
		return err
	}

	ingReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/v1/documents", &body)
	if err != nil {
		return err
	}
	ingReq.Header.Set("Content-Type", w.FormDataContentType())
	ingResp, err := (&http.Client{Timeout: 90 * time.Minute}).Do(ingReq)
	if err != nil {
		return fmt.Errorf("ingestion: %w", err)
	}
	defer ingResp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(ingResp.Body, 4<<10))
	if ingResp.StatusCode != http.StatusOK {
		return fmt.Errorf("ingestion: status %d: %s", ingResp.StatusCode, respBody)
	}
	log.Printf("ingestion ответ: %s", respBody)
	return nil
}
