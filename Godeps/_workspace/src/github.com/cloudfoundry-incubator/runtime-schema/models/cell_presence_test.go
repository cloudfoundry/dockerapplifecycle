package models_test

import (
	"github.com/cloudfoundry-incubator/docker-circus/Godeps/_workspace/src/github.com/cloudfoundry-incubator/runtime-schema/models"
	. "github.com/cloudfoundry-incubator/docker-circus/Godeps/_workspace/src/github.com/onsi/ginkgo"
	. "github.com/cloudfoundry-incubator/docker-circus/Godeps/_workspace/src/github.com/onsi/gomega"
)

var _ = Describe("CellPresence", func() {
	var cellPresence models.CellPresence

	var payload string

	BeforeEach(func() {
		cellPresence = models.NewCellPresence("some-id", "some-stack", "some-address", "some-zone")

		payload = `{
    "cell_id":"some-id",
    "stack": "some-stack",
    "rep_address": "some-address",
    "zone": "some-zone"
  }`
	})

	Describe("Validate", func() {
		Context("when cell presence is valid", func() {
			It("does not return an error", func() {
				Ω(cellPresence.Validate()).ShouldNot(HaveOccurred())
			})
		})
		Context("when cell presence is invalid", func() {
			Context("when cell id is invalid", func() {
				BeforeEach(func() {
					cellPresence.CellID = ""
				})
				It("returns an error", func() {
					err := cellPresence.Validate()
					Ω(err).Should(HaveOccurred())
					Ω(err.Error()).Should(ContainSubstring("cell_id"))
				})
			})
			Context("when stack is invalid", func() {
				BeforeEach(func() {
					cellPresence.Stack = ""
				})
				It("returns an error", func() {
					err := cellPresence.Validate()
					Ω(err).Should(HaveOccurred())
					Ω(err.Error()).Should(ContainSubstring("stack"))
				})
			})
			Context("when rep address is invalid", func() {
				BeforeEach(func() {
					cellPresence.RepAddress = ""
				})
				It("returns an error", func() {
					err := cellPresence.Validate()
					Ω(err).Should(HaveOccurred())
					Ω(err.Error()).Should(ContainSubstring("rep_address"))
				})
			})
		})
	})
	Describe("ToJSON", func() {
		It("should JSONify", func() {
			json, err := models.ToJSON(&cellPresence)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(string(json)).Should(MatchJSON(payload))
		})
	})

	Describe("FromJSON", func() {
		It("returns a CellPresence with correct fields", func() {
			decodedCellPresence := &models.CellPresence{}
			err := models.FromJSON([]byte(payload), decodedCellPresence)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(decodedCellPresence).Should(Equal(&cellPresence))
		})

		Context("with an invalid payload", func() {
			It("returns the error", func() {
				payload = "aliens lol"
				decodedCellPresence := &models.CellPresence{}
				err := models.FromJSON([]byte(payload), decodedCellPresence)

				Ω(err).Should(HaveOccurred())
			})
		})
	})
})
