package codegen

import (
	"fmt"
)

// nameRegistry tracks every identifier the generator will declare (or that
// the annotated source already declares) so collisions fail generation
// with a clear message instead of emitting uncompilable or silently
// overwritten code.
type nameRegistry struct {
	names map[string]string // identifier -> what declares it
}

func newNameRegistry() *nameRegistry {
	return &nameRegistry{names: map[string]string{}}
}

func (r *nameRegistry) claim(name, what string) error {
	if prev, taken := r.names[name]; taken {
		return fmt.Errorf("generated identifier %q for %s collides with %s; rename one of them", name, what, prev)
	}
	r.names[name] = what
	return nil
}

// validateNames claims every top-level identifier and client method the
// table's generated code declares. Entity struct names participate too:
// they live in the same package.
func validateNames(tv tableView, evs []*entityView) error {
	r := newNameRegistry()
	claims := []struct{ name, what string }{
		{tv.Client, "the table client type"},
		{"New" + tv.Client, "the client constructor"},
		{tv.Iface, "the sealed item interface"},
	}
	for _, pv := range tv.Partitions {
		claims = append(claims,
			struct{ name, what string }{pv.Name, "partition query method " + pv.Name},
			struct{ name, what string }{pv.QueryType, "partition query type " + pv.QueryType},
			struct{ name, what string }{pv.Collection, "collection type " + pv.Collection},
		)
	}
	for _, ev := range evs {
		e := ev.Entity
		claims = append(claims,
			struct{ name, what string }{e, "entity struct " + e},
			struct{ name, what string }{e + "Key", "the key struct of entity " + e},
			struct{ name, what string }{e + "Update", "the update builder of entity " + e},
			struct{ name, what string }{"Get" + e, "the Get method of entity " + e},
			struct{ name, what string }{"Put" + e, "the Put method of entity " + e},
			struct{ name, what string }{"Put" + e + "IfNotExists", "the PutIfNotExists method of entity " + e},
			struct{ name, what string }{"Delete" + e, "the Delete method of entity " + e},
			struct{ name, what string }{"Update" + e, "the Update method of entity " + e},
			struct{ name, what string }{"TransactPut" + e, "the TransactPut method of entity " + e},
			struct{ name, what string }{"TransactDelete" + e, "the TransactDelete method of entity " + e},
			struct{ name, what string }{"BatchGet" + ev.Batch.Plural, "the BatchGet method of entity " + e},
			struct{ name, what string }{"BatchPut" + ev.Batch.Plural, "the BatchPut method of entity " + e},
			struct{ name, what string }{"marshal" + e, "the marshal func of entity " + e},
			struct{ name, what string }{"unmarshal" + e, "the unmarshal func of entity " + e},
		)
		for _, kf := range ev.KeyFuncs {
			claims = append(claims, struct{ name, what string }{kf.Name, "the " + kf.Attr + " encoder of entity " + e})
		}
		for _, pv := range ev.Patterns {
			claims = append(claims,
				struct{ name, what string }{pv.Name, "pattern " + pv.Name + " of entity " + e},
				struct{ name, what string }{pv.QueryType, "the query type of pattern " + pv.Name},
			)
		}
	}
	// Reserved client methods that no entity or pattern may shadow.
	claims = append(claims,
		struct{ name, what string }{"TransactWrite", "the client TransactWrite method"},
		struct{ name, what string }{"DynamoDB", "the client DynamoDB accessor"},
		struct{ name, what string }{"TableName", "the client TableName accessor"},
	)
	for _, cl := range claims {
		if err := r.claim(cl.name, cl.what); err != nil {
			return err
		}
	}
	return nil
}
