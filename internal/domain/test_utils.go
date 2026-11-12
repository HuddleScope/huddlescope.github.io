package domain

// GetTestTeams returns a slice of teams for testing purposes.
func GetTestTeams() []Team {
	return []Team{
		{
			Slug:         "49ers",
			Name:         "San Francisco 49ers",
			ShortName:    "49ers",
			Abbreviation: "SF",
		},
		{
			Slug:         "cowboys",
			Name:         "Dallas Cowboys",
			ShortName:    "Cowboys",
			Abbreviation: "DAL",
		},
		{
			Slug:         "chiefs",
			Name:         "Kansas City Chiefs",
			ShortName:    "Chiefs",
			Abbreviation: "KC",
		},
	}
}
