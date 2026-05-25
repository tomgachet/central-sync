package main

func mergeDatasetEntitiesByID(
	regularEntities []map[string]interface{},
	deletedEntities []map[string]interface{},
) []map[string]interface{} {
	entityByID := make(map[string]map[string]interface{})

	for _, entity := range regularEntities {
		entityUUID, err := getEntityUUID(entity)
		if err != nil || entityUUID == "" {
			continue
		}
		entityByID[entityUUID] = entity
	}

	for _, entity := range deletedEntities {
		entityUUID, err := getEntityUUID(entity)
		if err != nil || entityUUID == "" {
			continue
		}
		entityByID[entityUUID] = entity
	}

	var merged []map[string]interface{}
	for _, entity := range entityByID {
		merged = append(merged, entity)
	}

	return merged
}