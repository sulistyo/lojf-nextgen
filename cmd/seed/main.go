package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/lojf/nextgen/internal/db"
	"github.com/lojf/nextgen/internal/models"
)

func main() {
	reset := flag.Bool("reset", false, "delete existing parents/children/classes/registrations before seeding")
	flag.Parse()

	if err := db.Init(); err != nil {
		log.Fatalf("db init: %v", err)
	}
	conn := db.Conn()

	if *reset {
		// Order matters due FK relations.
		conn.Exec("DELETE FROM registration_answers")
		conn.Exec("DELETE FROM registrations")
		conn.Exec("DELETE FROM class_questions")
		conn.Exec("DELETE FROM class_template_questions")
		conn.Exec("DELETE FROM class_templates")
		conn.Exec("DELETE FROM children")
		conn.Exec("DELETE FROM parents")
		conn.Exec("DELETE FROM classes")
	}

	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc)
	nextSunday := nextWeekday(now, time.Sunday, 9, 0, loc)
	nextSunday2 := nextSunday.AddDate(0, 0, 7)

	classes := []models.Class{
		{Name: "Kids Class A (Age 4-6)", Date: nextSunday, Capacity: 20, Description: "Bible story + art project"},
		{Name: "Kids Class B (Age 7-9)", Date: nextSunday, Capacity: 24, Description: "Praise + worksheet"},
		{Name: "Kids Class C (Age 10-12)", Date: nextSunday2, Capacity: 18, Description: "Small group + memory verse"},
	}
	for i := range classes {
		c := classes[i]
		if err := conn.Where("name = ? AND date = ?", c.Name, c.Date).FirstOrCreate(&c).Error; err != nil {
			log.Fatalf("seed class %q: %v", classes[i].Name, err)
		}
		classes[i] = c
	}

	parents := []models.Parent{
		{Name: "Budi Santoso", Phone: "+628111000001", Email: "budi@example.com"},
		{Name: "Rina Hartono", Phone: "+628111000002", Email: "rina@example.com"},
		{Name: "Andi Wijaya", Phone: "+628111000003", Email: "andi@example.com"},
	}

	type seededChild struct {
		ParentPhone string
		Child       models.Child
		ClassIdx    int
		Status      string
	}

	children := []seededChild{
		{ParentPhone: "+628111000001", Child: models.Child{Name: "Nadia", BirthDate: dateOnly(2018, 4, 12, loc), Gender: "Girl"}, ClassIdx: 0, Status: "confirmed"},
		{ParentPhone: "+628111000001", Child: models.Child{Name: "Rafael", BirthDate: dateOnly(2016, 11, 5, loc), Gender: "Boy"}, ClassIdx: 1, Status: "waitlisted"},
		{ParentPhone: "+628111000002", Child: models.Child{Name: "Mikha", BirthDate: dateOnly(2015, 3, 20, loc), Gender: "Boy"}, ClassIdx: 1, Status: "confirmed"},
		{ParentPhone: "+628111000003", Child: models.Child{Name: "Celine", BirthDate: dateOnly(2013, 8, 2, loc), Gender: "Girl"}, ClassIdx: 2, Status: "canceled"},
	}

	parentMap := map[string]models.Parent{}
	for _, p := range parents {
		pp := p
		if err := conn.Where("phone = ?", pp.Phone).FirstOrCreate(&pp).Error; err != nil {
			log.Fatalf("seed parent %s: %v", p.Phone, err)
		}
		parentMap[p.Phone] = pp
	}

	for _, row := range children {
		p, ok := parentMap[row.ParentPhone]
		if !ok {
			log.Fatalf("parent not found for %s", row.ParentPhone)
		}

		child := row.Child
		child.ParentID = p.ID
		if err := conn.Where("parent_id = ? AND name = ?", child.ParentID, child.Name).FirstOrCreate(&child).Error; err != nil {
			log.Fatalf("seed child %s: %v", child.Name, err)
		}

		reg := models.Registration{
			ParentID: p.ID,
			ChildID:  child.ID,
			ClassID:  classes[row.ClassIdx].ID,
			Status:   row.Status,
			Code:     fmt.Sprintf("DUMMY-%d-%d", child.ID, classes[row.ClassIdx].ID),
		}
		if row.Status == "confirmed" {
			ci := classes[row.ClassIdx].Date.Add(15 * time.Minute)
			reg.CheckInAt = &ci
		}

		if err := conn.Where("code = ?", reg.Code).FirstOrCreate(&reg).Error; err != nil {
			log.Fatalf("seed registration for %s: %v", child.Name, err)
		}
	}

	var pCount, cCount, clsCount, rCount int64
	conn.Model(&models.Parent{}).Count(&pCount)
	conn.Model(&models.Child{}).Count(&cCount)
	conn.Model(&models.Class{}).Count(&clsCount)
	conn.Model(&models.Registration{}).Count(&rCount)

	fmt.Printf("Seed complete. parents=%d children=%d classes=%d registrations=%d\n", pCount, cCount, clsCount, rCount)
}

func dateOnly(y int, m time.Month, d int, loc *time.Location) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

func nextWeekday(from time.Time, w time.Weekday, hour, min int, loc *time.Location) time.Time {
	days := (int(w) - int(from.Weekday()) + 7) % 7
	if days == 0 {
		days = 7
	}
	t := from.AddDate(0, 0, days)
	return time.Date(t.Year(), t.Month(), t.Day(), hour, min, 0, 0, loc)
}
