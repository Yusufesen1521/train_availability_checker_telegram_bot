package main

// --- İSTASYON MODELİ ---
type Station struct {
	ID         int      `json:"id"`
	UnitId     float64  `json:"unitId"`
	Name       string   `json:"name"`
	Code       string   `json:"stationCode"`
	TrainTypes []string `json:"stationTrainTypes"`
}

// --- İSTEK (REQUEST) MODELLERİ ---
type SearchRequest struct {
	SearchRoutes      []SearchRoute        `json:"searchRoutes"`
	PassengerCounts   []PassengerTypeCount `json:"passengerTypeCounts"`
	SearchReservation bool                 `json:"searchReservation"`
}

type SearchRoute struct {
	DepartureStationId   int    `json:"departureStationId"`
	DepartureStationName string `json:"departureStationName"`
	ArrivalStationId     int    `json:"arrivalStationId"`
	ArrivalStationName   string `json:"arrivalStationName"`
	DepartureDate        string `json:"departureDate"`
}

type PassengerTypeCount struct {
	Id    int `json:"id"`
	Count int `json:"count"`
}

// --- CEVAP (RESPONSE) MODELLERİ ---
type TrainResponse struct {
	TrainLegs []TrainLeg `json:"trainLegs"`
}

type TrainLeg struct {
	TrainAvailabilities []TrainAvailability `json:"trainAvailabilities"`
}

type TrainAvailability struct {
	Trains []Train `json:"trains"`
}

type Train struct {
	ID                int                 `json:"id"`
	Name              string              `json:"name"`
	TrainNumber       string              `json:"trainNumber"`
	MinPrice          *MinPrice           `json:"minPrice"`
	Segments          []SegmentElement    `json:"segments"`
	AvailableFareInfo []AvailableFareInfo `json:"availableFareInfo"`
}

type SegmentElement struct {
	DepartureTime int64 `json:"departureTime"`
	ArrivalTime   int64 `json:"arrivalTime"`
}

type AvailableFareInfo struct {
	CabinClasses []CabinClassElement `json:"cabinClasses"`
}

type CabinClassElement struct {
	AvailabilityCount float64                 `json:"availabilityCount"`
	CabinClass        *BookingClassCabinClass `json:"cabinClass"`
	MinPrice          float64                 `json:"minPrice"`
}

type BookingClassCabinClass struct {
	Name string `json:"name"`
}

type MinPrice struct {
	PriceAmount float64 `json:"priceAmount"`
	Currency    string  `json:"priceCurrency"`
}
