package result

type DetailsPayload interface {
	isResultDetails()
}

type ArtifactsPayload interface {
	isResultArtifacts()
}

type ItemPayload interface {
	isResultItem()
}
