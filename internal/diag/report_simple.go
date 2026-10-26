package diag

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Phobby/TuiSense/internal/i18n"
)

// Report presentation is independent of the current UI language and UX mode.
// Measurements and assessments remain the source of truth; no new fault
// thresholds are introduced by these formatters.
func reportLabel(c Category) string {
	labels := map[Category]string{CatSticks: "Analog çubuklar", CatTriggers: "Tetikler", CatButtons: "Tuşlar ve yön tuşları", CatHaptics: "Titreşim", CatMotion: "Hareket sensörleri", CatTouchpad: "Dokunmatik yüzey", CatAudioIn: "Mikrofon", CatAudioOut: "Hoparlör", CatBattery: "Pil verisi", CatConnection: "Bağlantı", CatDeadzone: "Ölü bölge önizlemesi", CatFirmware: "Firmware bilgisi"}
	if label := labels[c]; label != "" {
		return label
	}
	return "Diğer ölçümler"
}
func reportStatus(e ReportEntry) string {
	if entryWarning(e) {
		return "Uyarı"
	}
	switch e.Status {
	case Unsupported, Unavailable:
		return "Kullanılamıyor"
	case NotTested:
		return "Test edilmedi"
	case Testing, Incomplete:
		return "Eksik test"
	}
	if e.Completion == CompletionPartial || e.Completion == CompletionRunning {
		return "Eksik test"
	}
	if e.Status == Inconclusive {
		if e.Category == CatDeadzone {
			return "Normal gözlem"
		}
		return "Doğrulanamadı"
	}
	if e.Status == Passed {
		if e.Category == CatBattery || e.Category == CatFirmware || e.Category == CatDeadzone {
			return "Normal gözlem"
		}
		return "Başarılı"
	}
	return "Doğrulanamadı"
}
func entryWarning(e ReportEntry) bool {
	if e.Status == Unsupported || e.Status == Unavailable {
		return false
	}
	if warningStatus(e.Status) {
		return true
	}
	for _, a := range e.Assessments {
		if warningStatus(a.Status) {
			return true
		}
	}
	return false
}
func entryExplanation(e ReportEntry) string {
	var text string
	switch reportStatus(e) {
	case "Kullanılamıyor":
		text = "Bu cihaz veya bağlantıda ölçülemiyor; arıza göstergesi değil."
		if e.RequiredConnection == "USB" {
			text = "Bluetooth üzerinden kullanılamıyor; USB bağlantısı gerekiyor."
		}
	case "Test edilmedi":
		text = "Henüz sonuç yok."
	case "Eksik test":
		text = "Test tamamlanmadı."
	case "Doğrulanamadı":
		text = "Sonuç için yeterli kanıt yok."
	case "Uyarı":
		text = "Gözlenen davranış tekrar test edilmeli."
	default:
		text = "Tamamlanan ölçümlerde olağandışı davranış gözlenmedi."
	}
	if e.Result != nil && e.Status != Unsupported && e.Status != Unavailable {
		switch e.Category {
		case CatBattery:
			if len(e.Result.Measurements) > 0 {
				text = "Pil seviyesi okunabiliyor; pil sağlığı ölçülemiyor."
			}
		case CatFirmware:
			text = "Sürüm okundu; güncellik doğrulanmadı."
		case CatDeadzone:
			text = "Yalnızca önizleme; oyun girdisi değiştirilmedi."
		case CatButtons:
			if len(e.Components) > 0 {
				seen := 0
				for _, c := range e.Components {
					if c.Completion == CompletionDone {
						seen++
					}
				}
				text = fmt.Sprintf("%d/%d tuş algılandı.", seen, len(e.Components))
				if entryWarning(e) {
					text += " Uzun basılı durum gözlendi."
				}
			}
		case CatHaptics:
			if e.Result.UserConfirmed {
				text = "Hissedilen titreşim kullanıcı tarafından doğrulandı."
				if entryWarning(e) {
					text = "Bazı titreşim desenleri kullanıcı tarafından hissedilmedi."
				}
			}
		}
	}
	facts := map[string]float64{}
	for _, m := range e.KeyMeasurements {
		if validMeasurement(m) {
			facts[m.Name] = m.Value
		}
	}
	if count, ok := facts["Denenen bölge"]; ok {
		if total, ok := facts["Toplam bölge"]; ok {
			text = fmt.Sprintf("%.0f bölgeden %.0f tanesi denendi.", total, count)
		}
	}
	if count, ok := facts["Doğrulanan desen"]; ok {
		if total, ok := facts["Denenen desen"]; ok {
			text = fmt.Sprintf("Denenen %.0f desenin %.0f tanesi kullanıcı tarafından doğrulandı.", total, count)
		}
	}
	if percent, ok := facts["Önizleme ölü bölgesi"]; ok {
		text = "%" + number(percent) + " önizleme; oyun girdisi değiştirilmedi."
	}
	if e.Category == CatBattery && e.Result != nil {
		for _, m := range e.Result.Measurements {
			if (m.Name == "Reported level" || m.Name == i18n.Translate(i18n.Turkish, "Reported level")) && validMeasurement(m) {
				text = "Pil seviyesi: %" + number(m.Value) + "; pil sağlığı ölçülemiyor."
			}
		}
	}
	for _, a := range e.Assessments {
		if a.Test == "snapback" {
			for _, m := range a.Evidence {
				if m.Name == "Maximum overshoot" && validMeasurement(m) {
					text = "En yüksek geri sekme: %" + number(m.Value) + "."
				}
			}
		}
	}
	if entryWarning(e) && (e.Completion == CompletionPartial || e.Completion == CompletionRunning) {
		text += " Test tamamlanmadı."
	}
	return text
}
func reportAdvice(e ReportEntry) string {
	if e.Status == Unsupported {
		return ""
	}
	if e.Status == Unavailable {
		if e.RequiredConnection == "USB" {
			return "Ses testleri için kontrolcüyü USB üzerinden bağla."
		}
		return "Bu ölçüm için desteklenen bağlantı ve erişim koşullarını kontrol et."
	}
	if entryWarning(e) {
		switch e.Category {
		case CatSticks:
			return "Analog testini dokunmadan dinlenme ve ani bırakma adımlarıyla tekrarla; aynı hareketi oyun içinde ve ölü bölge önizlemesinde karşılaştır."
		case CatTriggers:
			return "Tetikleri tamamen bırakıp tam basarak testi tekrarla; analog/dijital eşlemeyi ve kontrolcünün giriş modunu kontrol et."
		case CatButtons:
			return "Tuşları tamamen bırakıp yeniden dene; dokunmadan basılı görünüp görünmediğini kontrol et."
		case CatConnection:
			return "Bağlantı testini tekrarla; kablolu bağlantıda başka bir USB portu veya kablo dene."
		case CatBattery:
			return "Güncel pil seviyesini ve şarj durumunu kontrol et."
		default:
			return entryLabel(e) + " testini tekrarla; gözlemin tekrarlanıp tekrarlanmadığını kontrol et."
		}
	}
	switch reportStatus(e) {
	case "Eksik test", "Test edilmedi":
		return entryLabel(e) + " testinin yönlendirmelerini tamamla."
	case "Doğrulanamadı":
		return entryLabel(e) + " ölçümünü tekrarla; mevcut veri kesin sonuç için yetersiz."
	}
	return ""
}
func reportImpact(c Category, snap bool) string {
	switch c {
	case CatSticks:
		if snap {
			return "Ani bırakmada kısa süreli ters yön girdisi gözlendi. Küçük ölü bölge kullanan oyunlarda kamera veya karakter istemeden hareket edebilir."
		}
		return "Dinlenme konumu veya hareket dönüşünde beklenmeyen girdi gözlendi. Küçük ölü bölge kullanan oyunlarda istemsiz harekete yol açabilir."
	case CatTriggers:
		return "Gözlenen tetik tepkisi ek kontrol gerektiriyor. Eksik basma, giriş modu veya eşleme sonucu etkileyebilir; oyunlarda tam tepki alınamayabilir."
	case CatButtons:
		return "Uzun süre basılı girdi gözlendi. Tuşa dokunulmadığında da sürerse oyundaki eylem devam edebilir; bilinçli uzun basma da aynı sonucu oluşturur."
	case CatHaptics:
		return "Kullanıcı bazı titreşimleri hissetmediğini bildirdi; oyunlardaki titreşim geri bildirimi de zayıf veya eksik olabilir. Bu test motor gücünü bir ölçüm cihazıyla doğrulamaz."
	case CatConnection:
		return "Bağlantı veya veri okuma gözlemi tekrar kontrol edilmeli. Tekrarlanan kesintiler oyun girdisini durdurabilir."
	default:
		return "Ölçüm veya kullanıcı gözlemi ek kontrol gerektiriyor; bu test tek başına fiziksel nedeni belirleyemez."
	}
}
func reportDate(t time.Time) string {
	months := [...]string{"", "Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}
	t = t.Local()
	return fmt.Sprintf("%d %s %d %02d:%02d", t.Day(), months[t.Month()], t.Year(), t.Hour(), t.Minute())
}
func reportConnection(value string) string {
	if strings.Contains(strings.ToLower(value), "usb") {
		return "USB"
	}
	if strings.EqualFold(value, "bluetooth") {
		return "Bluetooth"
	}
	if value == "" || strings.EqualFold(value, "unknown") {
		return "Bilinmiyor"
	}
	return i18n.Translate(i18n.Turkish, value)
}
func known(value string) string {
	if value == "" {
		return "Bilinmiyor"
	}
	return value
}
func number(v float64) string { return strings.ReplaceAll(fmt.Sprintf("%.1f", v), ".", ",") }

// Only explicitly named, useful measurements enter the short report. Raw
// counts, mapping identifiers and unrecognized driver fields stay technical.
func measurementLabel(names map[string]string, key string) (string, bool) {
	if name, ok := names[key]; ok {
		return name, true
	}
	for source, name := range names {
		if i18n.Translate(i18n.Turkish, source) == key {
			return name, true
		}
	}
	return "", false
}
func importantMeasurements(e ReportEntry) []string {
	var out []string
	seen := map[string]bool{}
	add := func(name string, m Measurement) {
		if !validMeasurement(m) || seen[name] {
			return
		}
		seen[name] = true
		unit := m.Unit
		if strings.Contains(unit, "%") {
			unit = "%"
		}
		out = append(out, fmt.Sprintf("%s: %s %s", name, number(m.Value), unit))
	}
	names := map[string]string{"Initial center offset": "İlk merkez sapması", "Average resting deviation": "Ortalama dinlenme sapması", "Maximum resting deviation": "En yüksek dinlenme sapması", "Resting noise": "Girdi titreşimi", "Return-to-center shift": "Merkeze dönüş farkı", "Movement coverage": "Hareket kapsamı", "Resting value": "Dinlenme değeri", "Value after release": "Bırakma sonrası değer", "Maximum observed response": "En yüksek tepki", "Continuous hold": "Kesintisiz basılı süre", "Maximum overshoot": "En yüksek geri sekme", "Average overshoot": "Ortalama geri sekme", "Releases beyond preview": "Önizleme sınırını aşan bırakma", "Sudden releases": "Ani bırakma sayısı", "Preview deadzone": "Önizleme ölü bölgesi", "Reported level": "Pil seviyesi", "Interruptions": "Bağlantı kesintisi", "Read errors": "Okuma hatası", "Reports missing in sequence": "Eksik girdi paketi", "Peak level": "En yüksek ses seviyesi", "Quietest level": "En düşük ses seviyesi", "Resting acceleration magnitude (nominal scale)": "Dinlenme ivmesi (nominal ölçek)"}
	assessments := append([]Assessment(nil), e.Assessments...)
	if entryWarning(e) {
		var warnings []Assessment
		for _, a := range assessments {
			if warningStatus(a.Status) {
				warnings = append(warnings, a)
			}
		}
		if len(warnings) > 0 {
			assessments = warnings
		}
	}
	for _, a := range assessments {
		for _, m := range a.Evidence {
			if name, ok := measurementLabel(names, m.Name); ok {
				add(i18n.Translate(i18n.Turkish, a.Component)+" — "+name, m)
			}
		}
	}
	if e.Result != nil {
		for _, m := range e.Result.Measurements {
			if name, ok := measurementLabel(names, m.Name); ok {
				add(name, m)
			} else if m.Name == i18n.Translate(i18n.Turkish, "Reported level") {
				add("Pil seviyesi", m)
			}
		}
	}
	for _, m := range e.KeyMeasurements {
		add(m.Name, m)
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}
func entryLabel(e ReportEntry) string {
	if (e.Category == CatSticks || e.Category == CatTriggers) && len(e.Assessments) > 0 && (e.Assessments[0].Test == "snapback" || e.Assessments[0].Test == "Analog sticks / drift" || e.Assessments[0].Test == "Triggers") {
		return i18n.Translate(i18n.Turkish, e.Assessments[0].Component)
	}
	return reportLabel(e.Category)
}

// Split only measured components, retaining category coverage on each row.
func (r Report) displayEntries() []ReportEntry {
	var rows []ReportEntry
	for _, e := range r.Entries {
		if len(r.Entries) > 1 && e.Status == Unsupported {
			continue
		}
		if e.Category != CatSticks && e.Category != CatTriggers {
			rows = append(rows, e)
			continue
		}
		groups := map[string]int{}
		var parts []ReportEntry
		for _, a := range e.Assessments {
			if a.Test != "snapback" && a.Test != "Analog sticks / drift" && a.Test != "Triggers" {
				continue
			}
			index, ok := groups[a.Component]
			if !ok {
				part := e
				part.Assessments = nil
				part.Status = a.Status
				for _, component := range e.Components {
					if component.Component == a.Component {
						part.Completion = component.Completion
					}
				}
				index = len(parts)
				groups[a.Component] = index
				parts = append(parts, part)
			}
			parts[index].Assessments = append(parts[index].Assessments, a)
			if warningStatus(a.Status) || (a.Status == Inconclusive && !warningStatus(parts[index].Status)) {
				parts[index].Status = a.Status
			}
		}
		if len(parts) == 0 {
			rows = append(rows, e)
		} else {
			rows = append(rows, parts...)
		}
	}
	return rows
}
func (r Report) simpleActions() []string {
	var out []string
	seen := map[string]bool{}
	for _, phase := range []int{0, 1, 2} {
		for _, e := range r.displayEntries() {
			priority := 2
			if entryWarning(e) {
				priority = 0
			} else if e.Completion == CompletionPartial || e.Completion == CompletionNone || e.Completion == CompletionRunning {
				priority = 1
			}
			if priority != phase {
				continue
			}
			a := reportAdvice(e)
			if a != "" && !seen[a] {
				seen[a] = true
				out = append(out, a)
			}
		}
	}
	return out
}
func (r Report) simpleIssues(markdown bool) string {
	var b strings.Builder
	for _, e := range r.displayEntries() {
		if !entryWarning(e) {
			continue
		}
		title := "Gözlem tekrar kontrol edilmeli"
		if e.Category == CatSticks {
			for _, a := range e.Assessments {
				if a.Test == "Analog sticks / drift" && a.Status == PossibleIssue {
					title = "Tekrarlanan dinlenme sapması"
				}
			}
		}
		snap := false
		metrics := importantMeasurements(e)
		for _, a := range e.Assessments {
			if a.Test == "snapback" && warningStatus(a.Status) {
				title = "Ani bırakmada geri sekme"
				snap = true
				values := map[string]float64{}
				for _, m := range a.Evidence {
					if validMeasurement(m) {
						values[m.Name] = m.Value
					}
				}
				metrics = nil
				for _, pair := range [][2]string{{"Maximum overshoot", "En yüksek geri sekme"}, {"Average overshoot", "Ortalama geri sekme"}} {
					if v, ok := values[pair[0]]; ok {
						metrics = append(metrics, pair[1]+": %"+number(v))
					}
				}
				count, cok := values["Sudden releases"]
				cross, xok := values["Releases beyond preview"]
				boundary, bok := values["Preview deadzone"]
				if cok && xok && bok {
					metrics = append(metrics, fmt.Sprintf("%.0f ani bırakmanın %.0f tanesinde %%%s önizleme sınırı aşıldı.", count, cross, number(boundary)))
				}
				break
			}
		}
		if markdown {
			b.WriteString("### ⚠️ ")
		} else {
			b.WriteString("⚠ ")
		}
		fmt.Fprintf(&b, "%s — %s\n\n", entryLabel(e), title)
		if len(metrics) > 3 {
			metrics = metrics[:3]
		}
		for _, m := range metrics {
			fmt.Fprintf(&b, "- %s\n", m)
		}
		if e.Completion == CompletionPartial || e.Completion == CompletionRunning {
			b.WriteString("- Test tamamlanmadı; mevcut gözlem korunuyor.\n")
		}
		fmt.Fprintf(&b, "\nNe anlama geliyor?\n%s Bu bulgu tek başına donanım arızasını kanıtlamaz.\n\nÖneri:\n%s\n\n", reportImpact(e.Category, snap), reportAdvice(e))
	}
	return b.String()
}
func (r Report) WarningsText() string {
	s := "TuiSense — Tanılama Uyarıları\n\nCihaz: " + r.Device.Model + "\n\n"
	if r.Device.Simulated {
		s += "SİMÜLASYON: Gerçek donanım ölçümü değildir.\n\n"
	}
	issues := r.simpleIssues(false)
	if issues == "" {
		issues = "Önemli bir uyarı gözlenmedi. Eksik veya desteklenmeyen testler başarılı sayılmaz.\n"
	}
	return s + issues
}
func (r Report) SummaryText() string {
	if len(r.Entries) == 1 {
		return r.TestText()
	}
	return r.Text()
}
func (r Report) Text() string { var b strings.Builder; _ = r.WriteText(&b); return b.String() }
func (r Report) TestText() string {
	var b strings.Builder
	if r.Device.Simulated {
		b.WriteString("SİMÜLASYON: Gerçek donanım ölçümü değildir.\n\n")
	}
	for _, e := range r.displayEntries() {
		fmt.Fprintf(&b, "Test: %s\nSonuç: %s\n\n", entryLabel(e), reportStatus(e))
		metrics := importantMeasurements(e)
		if e.Category == CatFirmware && r.Device.Firmware != "" {
			metrics = append(metrics, "Firmware sürümü: "+r.Device.Firmware)
		}
		if len(metrics) > 0 {
			b.WriteString("Ölçümler:\n")
			for _, m := range metrics {
				fmt.Fprintf(&b, "- %s\n", m)
			}
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "Değerlendirme:\n%s\n", entryExplanation(e))
		if entryWarning(e) {
			fmt.Fprintf(&b, "%s Tek başına donanım arızası kanıtı değildir.\n", reportImpact(e.Category, false))
		}
		if a := reportAdvice(e); a != "" {
			fmt.Fprintf(&b, "\nÖneri:\n%s\n", a)
		}
	}
	return b.String()
}
func (r Report) writeDocument(w io.Writer, markdown bool) error {
	var b strings.Builder
	heading := func(level int, title string) {
		if markdown {
			b.WriteString(strings.Repeat("#", level) + " ")
		}
		b.WriteString(title + "\n\n")
	}
	heading(1, "TuiSense — Kontrolcü Sağlık Raporu")
	for _, field := range [][2]string{{"Cihaz", r.Device.Model}, {"Bağlantı", reportConnection(r.Device.Connection)}, {"Firmware", known(r.Device.Firmware)}, {"Test tarihi", reportDate(r.GeneratedAt)}} {
		if markdown {
			fmt.Fprintf(&b, "**%s:** %s  \n", field[0], field[1])
		} else {
			fmt.Fprintf(&b, "%s: %s\n", field[0], field[1])
		}
	}
	b.WriteString("\n")
	heading(2, "Genel Sonuç")
	warnings := 0
	var normal []string
	for _, e := range r.displayEntries() {
		if entryWarning(e) {
			warnings++
		}
		if e.Status == Passed && e.Completion == CompletionDone && e.Category != CatFirmware && e.Category != CatBattery {
			normal = append(normal, entryLabel(e))
		}
	}
	if r.Device.Simulated {
		b.WriteString("SİMÜLASYON: Bu rapor gerçek donanım ölçümü içermez. ")
	}
	if warnings > 0 {
		fmt.Fprintf(&b, "Dikkat gerektiren %d bileşen bulgusu var; ayrıntılar aşağıda. ", warnings)
	} else {
		b.WriteString("Tamamlanan gözlemlerde önemli bir uyarı bulunmadı. ")
	}
	if len(normal) > 0 {
		fmt.Fprintf(&b, "Normal gözlenen testler: %s. ", strings.Join(normal, ", "))
	}
	if r.Coverage.Partial {
		b.WriteString("Bazı testler tamamlanmadığından tüm bileşenler hakkında sonuç verilemiyor.")
	} else {
		b.WriteString("Uygulanabilir testlerin kayıtlı adımları tamamlandı; bu durum tek başına kusursuz donanım anlamına gelmez.")
	}
	b.WriteString("\n\n")
	if warnings > 0 {
		heading(2, "Olası Sorunlar")
		b.WriteString(r.simpleIssues(markdown))
	}
	heading(2, "Test Sonuçları")
	if markdown {
		b.WriteString("| Bileşen | Sonuç | Açıklama |\n|---|---|---|\n")
	} else {
		b.WriteString("Bileşen — Sonuç — Açıklama\n")
	}
	for _, e := range r.displayEntries() {
		if markdown {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", mdCell(entryLabel(e)), reportStatus(e), mdCell(entryExplanation(e)))
		} else {
			fmt.Fprintf(&b, "%s — %s — %s\n", entryLabel(e), reportStatus(e), entryExplanation(e))
		}
	}
	b.WriteString("\n")
	heading(2, "Önerilen İşlemler")
	actions := r.simpleActions()
	if len(actions) == 0 {
		b.WriteString("Tamamlanan testlere göre acil işlem gerekmiyor.\n")
	}
	for i, a := range actions {
		fmt.Fprintf(&b, "%d. %s\n", i+1, a)
	}
	b.WriteString("\n")
	heading(2, "Son Değerlendirme")
	b.WriteString("Bu rapor kaydedilen gözlemleri özetler; olası sorunlar tekrarlanan testlerle değerlendirilmelidir. ")
	if r.Coverage.Partial {
		b.WriteString("Eksik testler nedeniyle kesin donanım sağlığı sonucu verilemiyor. ")
	}
	if r.Coverage.Unsupported+r.Coverage.Unavailable > 0 {
		b.WriteString("Desteklenmeyen veya erişilemeyen ölçümler arıza olarak değerlendirilmez.")
	}
	b.WriteString("\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// Technical reports aggregate duplicate events and cap the number of distinct
// rows. Original session events remain available in the raw log view.
func boundedReportEvents(events []Event) []Event {
	var out []Event
	seen := map[string]bool{}
	for i := len(events) - 1; i >= 0 && len(out) < 32; i-- {
		e := events[i]
		key := string(e.Category) + "\x00" + e.Message
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// Store concise facts with the snapshot so history never reads current state.
func setKeyMeasurements(s *Session, e *ReportEntry) {
	if e.Result == nil || e.Status == Unsupported || e.Status == Unavailable {
		return
	}
	switch e.Category {
	case CatTouchpad:
		if s.Touch != nil {
			covered, total := s.Touch.Coverage()
			e.KeyMeasurements = []Measurement{{"Denenen bölge", float64(covered), ""}, {"Toplam bölge", float64(total), ""}}
		}
	case CatHaptics:
		if s.Haptics != nil {
			confirmed := 0
			for _, felt := range s.Haptics.Feedback {
				if felt {
					confirmed++
				}
			}
			e.KeyMeasurements = []Measurement{{"Doğrulanan desen", float64(confirmed), ""}, {"Denenen desen", float64(len(s.Haptics.Feedback)), ""}}
		}
	case CatButtons:
		if len(e.Components) > 0 {
			count := 0
			for _, c := range e.Components {
				if c.Completion == CompletionDone {
					count++
				}
			}
			e.KeyMeasurements = []Measurement{{"Algılanan tuş", float64(count), ""}, {"Toplam tuş", float64(len(e.Components)), ""}}
		}
	case CatDeadzone:
		for _, m := range e.Result.Measurements {
			if m.Name == "Preview deadzone" {
				m.Name = "Önizleme ölü bölgesi"
				e.KeyMeasurements = append(e.KeyMeasurements, m)
			}
		}
	}
}
