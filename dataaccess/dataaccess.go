package dataaccess

import (
	"log"
	"strings"
	"time"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// The DataAccess interface defines how data is written to the data store.
type DataAccess interface {
	ListProfiles(emailAddress string) ([]Profile, error)
	GetProfile(emailAddress string) (*Profile, bool, error)
	UpdateProfile(update *ProfileUpdate) (*Profile, error)
	DeleteProfile(emailAddress string) (bool, error)
	ListSkillTags() ([]string, error)
	AddSkillTags(tags []string) error
	DeleteSkillTags(tags []string) error
	GetOrCreateConfiguration() (Configuration, error)
	DeleteConfiguration() error
}

// MongoDataAccess provides access to the data structures.
type MongoDataAccess struct {
	connectionString string
	databaseName     string
}

// NewMongoDataAccess creates an instance of the MongoDataAccess type.
func NewMongoDataAccess(connectionString string, databaseName string) DataAccess {
	return &MongoDataAccess{connectionString, databaseName}
}

// GetProfile returns a Profile by the email address of the person.
func (da MongoDataAccess) GetProfile(emailAddress string) (*Profile, bool, error) {
	session, err := mgo.Dial(da.connectionString)
	if err != nil {
		log.Print("Failed to connect to MongoDB.", err)
		return nil, false, err
	}
	defer session.Close()

	c := session.DB(da.databaseName).C("profiles")

	result := NewProfile()
	result.EmailAddress = emailAddress
	err = c.FindId(emailAddress).One(result)

	if err == mgo.ErrNotFound {
		log.Printf("Failed to find a profile with email %s.", emailAddress)
		return result, false, nil
	}

	return result, true, nil
}

// UpdateProfile updates a person's profile and returns the newly created
// or updated profile.
func (da MongoDataAccess) UpdateProfile(update *ProfileUpdate) (*Profile, error) {
	log.Printf("Updating profile for %s", update.EmailAddress)

	session, err := mgo.Dial(da.connectionString)
	if err != nil {
		log.Print("Failed to connect to MongoDB.", err)
		return nil, err
	}
	defer session.Close()

	c := session.DB(da.databaseName).C("profiles")

	profile, found, err := da.GetProfile(update.EmailAddress)

	if err != nil {
		log.Print(err)
		return nil, err
	}

	if found {
		log.Printf("Found existing profile for %s", update.EmailAddress)
	} else {
		log.Printf("New profile found for %s", update.EmailAddress)
	}

	if len(profile.Skills) > 0 {
		// Move current skills to history, if it's an update to an existing profile.
		sl := SkillLevel{
			Date:   profile.LastUpdated,
			Skills: profile.Skills,
		}

		profile.SkillsHistory = append(profile.SkillsHistory, sl)
	}

	for _, skill := range update.Skills {
		skill.Skill = strings.ToLower(skill.Skill)
	}

	profile.Skills = update.Skills
	profile.Availability = update.Availability
	profile.Version++
	profile.LastUpdated = time.Unix(time.Now().Unix(), 0)
	profile.Domain = getDomain(update.EmailAddress)

	_, err = c.UpsertId(profile.EmailAddress, profile)

	if err != nil {
		log.Print(err)
		return nil, err
	}

	return profile, nil
}

// ListSkillTags lists the skills used before.
func (da MongoDataAccess) ListSkillTags() ([]string, error) {
	session, err := mgo.Dial(da.connectionString)
	if err != nil {
		log.Print("Failed to connect to MongoDB. ", err)
		return nil, err
	}
	defer session.Close()

	c := session.DB(da.databaseName).C("skills")

	var results []SkillTag
	err = c.Find(nil).All(&results)

	if err != nil {
		log.Print("Failed to list skill tags. ", err)
		return nil, nil
	}

	skillTags := make([]string, len(results), len(results))
	for idx, tag := range results {
		skillTags[idx] = tag.Name
	}

	return skillTags, nil
}

// AddSkillTags adds a skill tag to the list.
func (da MongoDataAccess) AddSkillTags(tags []string) error {
	session, err := mgo.Dial(da.connectionString)
	if err != nil {
		log.Print("Failed to connect to MongoDB.", err)
		return err
	}
	defer session.Close()

	c := session.DB(da.databaseName).C("skills")

	for _, tag := range tags {
		_, err = c.UpsertId(tag, SkillTag{CleanTag(tag)})

		if err != nil {
			return err
		}
	}

	return nil
}

// DeleteProfile removes a profile specified by email address.
func (da MongoDataAccess) DeleteProfile(emailAddress string) (bool, error) {
	session, err := mgo.Dial(da.connectionString)
	if err != nil {
		log.Print("Failed to connect to MongoDB.", err)
		return false, err
	}
	defer session.Close()

	err = session.DB(da.databaseName).C("profiles").RemoveId(emailAddress)

	if err != nil {
		return false, err
	}

	return true, nil
}

// ListProfiles lists all of the profiles that the user has access to (filtered by domain).
func (da MongoDataAccess) ListProfiles(emailAddress string) ([]Profile, error) {
	session, err := mgo.Dial(da.connectionString)
	if err != nil {
		log.Print("Failed to connect to MongoDB.", err)
		return nil, err
	}
	defer session.Close()

	var results []Profile
	err = session.DB(da.databaseName).C("profiles").Find(bson.M{"domain": getDomain(emailAddress)}).All(&results)

	if err != nil {
		log.Print("Failed to list profiles.", err)
		return nil, err
	}

	return results, nil
}

func getDomain(emailAddress string) string {
	return strings.ToLower(strings.Split(emailAddress, "@")[1])
}

// DeleteSkillTags deletes a set of tags from the database.
func (da MongoDataAccess) DeleteSkillTags(tags []string) error {
	session, err := mgo.Dial(da.connectionString)
	if err != nil {
		log.Print("Failed to connect to MongoDB.", err)
		return err
	}
	defer session.Close()

	for _, tag := range tags {
		err = session.DB(da.databaseName).C("skills").RemoveId(tag)

		if err != nil && err != mgo.ErrNotFound {
			return err
		}
	}
	return nil
}

// CleanTag lowercases input tags and replaces spaces with hyphens.
func CleanTag(tag string) string {
	return strings.Replace(strings.ToLower(tag), " ", "-", -1)
}

func (da MongoDataAccess) getConfiguration() (Configuration, error) {
	session, err := mgo.Dial(da.connectionString)
	if err != nil {
		log.Print("Failed to connect to MongoDB. ", err)
		return Configuration{}, err
	}
	defer session.Close()

	configuration := NewConfiguration(nil)
	err = session.DB(da.databaseName).C("configuration").FindId("configuration").One(&configuration)

	return *configuration, err
}

func (da MongoDataAccess) attemptToCreateConfiguration() error {
	session, err := mgo.Dial(da.connectionString)
	if err != nil {
		log.Print("Failed to connect to MongoDB. ", err)
		return err
	}
	defer session.Close()

	configuration := NewConfiguration(createSessionEncryptionKey())
	return session.DB(da.databaseName).C("configuration").Insert(configuration)
}

// GetOrCreateConfiguration gets configuration from the database, or creates new configuration.
func (da MongoDataAccess) GetOrCreateConfiguration() (Configuration, error) {
	configuration, err := da.getConfiguration()

	if err == mgo.ErrNotFound {
		da.attemptToCreateConfiguration()
		return da.getConfiguration()
	}

	return configuration, err
}

// DeleteConfiguration deletes the configuration record.
func (da MongoDataAccess) DeleteConfiguration() error {
	session, err := mgo.Dial(da.connectionString)
	if err != nil {
		log.Print("Failed to connect to MongoDB. ", err)
		return err
	}
	defer session.Close()

	return session.DB(da.databaseName).C("configuration").DropCollection()
}
