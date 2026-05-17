package enum

type CommissionStatus string

const (
	Pending CommissionStatus = "PENDING"
	Charged CommissionStatus = "CHARGED"
	Failed  CommissionStatus = "FAILED"
)

func (cs CommissionStatus) Value() string {
	return string(cs)
}

func (cs CommissionStatus) IsValid() bool {
	switch cs {
	case Pending, Charged, Failed:
		return true
	}
	return false
}

func (CommissionStatus) Values() []string {
	return []string{
		string(Pending),
		string(Charged),
		string(Failed),
	}
}
