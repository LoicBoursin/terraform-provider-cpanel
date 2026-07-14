# Examples

This directory contains examples used for Registry documentation. They can
also be adapted for manual Terraform testing.

The documentation generator reads these locations:

- `provider/provider.tf` for the provider index;
- `data-sources/<name>/data-source.tf` for a data source;
- `resources/<name>/resource.tf` for a resource;
- `resources/<name>/import.sh` for import examples.

Do not place real API tokens or passwords in these files.
