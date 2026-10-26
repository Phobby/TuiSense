package diag

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Phobby/TuiSense/internal/i18n"
)

// Technical reports retain numeric evidence and mapping metadata. Structured
// JSON also preserves the original rule records for reproducible debugging.
func (r Report) TechnicalText() string {
	var b strings.Builder
	_ = r.WriteTechnicalText(&b)
	return b.String()
}
func (r Report) WriteTechnicalText(w io.Writer) error     { return r.writeTechnicalDocument(w, false) }
func (r Report) WriteTechnicalMarkdown(w io.Writer) error { return r.writeTechnicalDocument(w, true) }
func (r Report) writeTechnicalDocument(w io.Writer, markdown bool) error {
	var b strings.Builder
	b.WriteString("TuiSense — Teknik Tanılama Raporu\n\n")
	fmt.Fprintf(&b, "Cihaz: %s\nBağlantı: %s\nFirmware: %s\nTest tarihi: %s\nYazılım: %s\nŞema: %s\nAlgoritma: %s\nEşleme: %s\nGiriş modu: %s\nYöntem: %s\n", r.Device.Model, r.Device.Connection, known(r.Device.Firmware), reportDate(r.GeneratedAt), r.Version, r.SchemaVersion, r.Algorithm, r.Mapping, known(r.InputMode), r.Method)
	if r.Device.Simulated {
		b.WriteString("SİMÜLASYON: Gerçek donanım ölçümü değildir.\n")
	}
	fmt.Fprintf(&b, "\nTetik eşlemesi: %s\n", r.InputMapping.TriggerKind)
	for _, a := range r.InputMapping.Axes {
		fmt.Fprintf(&b, "%s / %s: ham aralık %.0f…%.0f; merkez %.2f; geçerli %v; ters %v\n", i18n.Translate(i18n.Turkish, a.Label), a.Name, a.Range.Min, a.Range.Max, a.Range.Center, a.Valid, a.Range.Invert)
	}
	for _, e := range r.Entries {
		fmt.Fprintf(&b, "\n%s — %s\n%s\n", reportLabel(e.Category), reportStatus(e), entryExplanation(e))
		if e.Result != nil {
			fmt.Fprintf(&b, "Ölçüm zamanı: %s\n", e.Result.At.Format(time.RFC3339))
			if e.Result.UserConfirmed {
				b.WriteString("Kullanıcı doğrulaması; cihazla ölçülmüş fiziksel performans değildir.\n")
			}
		}
		seen := map[string]bool{}
		writeMeasurement := func(m Measurement) {
			if !validMeasurement(m) {
				return
			}
			key := fmt.Sprintf("%s/%g/%s", m.Name, m.Value, m.Unit)
			if seen[key] {
				return
			}
			seen[key] = true
			if markdown {
				fmt.Fprintf(&b, "| %s | %.4f | %s |\n", mdCell(i18n.Translate(i18n.Turkish, m.Name)), m.Value, mdCell(m.Unit))
			} else {
				fmt.Fprintf(&b, "%s: %.4f %s\n", i18n.Translate(i18n.Turkish, m.Name), m.Value, m.Unit)
			}
		}
		if markdown {
			b.WriteString("\n| Ölçüm | Değer | Birim |\n|---|---:|---|\n")
		}
		if e.Result != nil {
			for _, m := range e.Result.Measurements {
				writeMeasurement(m)
			}
		}
		for _, a := range e.Assessments {
			for _, m := range a.Evidence {
				if seen[fmt.Sprintf("%s/%g/%s", m.Name, m.Value, m.Unit)] {
					continue
				}
				m.Name = i18n.Translate(i18n.Turkish, a.Component) + " / " + m.Name
				writeMeasurement(m)
			}
		}
		b.WriteString("Karşılaştırma ölçütü: Kaydedilen test protokolü ve sürücünün bildirdiği giriş aralığı; eşikler üretici arıza sınırı değildir.\n")
		if e.Category == CatSticks {
			b.WriteString("Dinlenme sapması, tekrar ölçümü ve hareket kapsamı birlikte değerlendirilir. Geri sekme seçili önizleme sınırıyla karşılaştırılır; düşük örnekleme kısa hareketleri kaçırabilir.\n")
		}
		if e.Category == CatTriggers {
			b.WriteString("Dijital tetiklere analog aralık testi uygulanmaz. Eksik basma veya eşleme farkı donanım arızası sayılmaz.\n")
		}
		if a := reportAdvice(e); a != "" {
			fmt.Fprintf(&b, "Öneri: %s\n", a)
		}
	}
	b.WriteString("\nOlay özeti\nAynı kategoride aynı mesaj bir kez gösterilir; en fazla 32 farklı olay. Tam ham kayıtlar uygulamanın Ham Kayıtlar ekranındadır.\n")
	events := boundedReportEvents(r.Events)
	for _, e := range events {
		fmt.Fprintf(&b, "%s [%s] %s\n", e.At.Format(time.RFC3339), e.Category, i18n.Translate(i18n.Turkish, e.Message))
	}
	fmt.Fprintf(&b, "Gösterilen olay: %d / %d kayıt.\n", len(events), len(r.Events))
	b.WriteString("\nBelirsizlik: Tamamlanan test kusursuz donanım kanıtı değildir. Uyarı kesin arıza değildir. Özgün değerlendirme alanları ve kanıtlar JSON dışa aktarımında korunur.\n")
	_, err := io.WriteString(w, b.String())
	return err
}
