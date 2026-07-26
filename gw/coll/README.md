# orm/coll — Collection & Relations

## Terms

- **Foreign Key (FK):** DB design — which table holds the foreign key column to link to the target relation model.
- **Foreign Key Field:** The field of the model corresponding to the FK column of the table.
- **Relation Owner:** Who defines the relation as a field. The subject of the relation.
- **Relation Field:** The field on the Relation Owner to get the target relation model.

## Relations

### BelongsTo

Child BelongsTo Parent

- FK on Child table
- Relation Owner: Child (subject of "Child BelongsTo Parent")
- Relation Field: on the Child — `Child.Parent` gives the parent

### HasMany

Parent HasMany Children

- FK on Child table
- Relation Owner: Parent (subject of "Parent HasMany Children")
- Relation Field: on the Parent — `Parent.Children` gives the children

### HasOne

Parent HasOne Child

- FK on Child table (same as HasMany)
- Relation Owner: Parent (subject of "Parent HasOne Child")
- Relation Field: on the Parent — `Parent.Child` gives the child
- HasMany with cardinality limited to 1
