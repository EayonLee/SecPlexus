#!/usr/bin/env python3
"""Validate actual published fixtures against the compiled OCSF 1.10.0 schema."""

import argparse
import json
from pathlib import Path

from jsonschema import Draft202012Validator


TYPES = {
    "integer_t": "integer", "long_t": "integer", "float_t": "number", "string_t": "string",
    "boolean_t": "boolean", "json_t": "object", "object_t": "object",
}
# 平台扩展只有已声明的簿记字段能通过。其余字段由上游类和对象定义决定。
EXTENSION = {
    "type": "object",
    "additionalProperties": False,
    "properties": {
        **{name: {"type": "string"} for name in (
            "input_id", "source_record_id", "vendor", "product", "branch_id",
            "parsing_rule_id", "contract_version", "time_source", "parse.outcome",
        )},
        **{name: {"type": "integer"} for name in (
            "source_revision", "parsing_rule_version", "candidate_class_uid", "received_at", "original_bytes",
        )},
        "parse.issues": {"type": "array", "items": {"type": "object"}},
        "parse.error": {"type": "object"},
        "truncated": {"type": "boolean"},
    },
    "required": ["input_id", "contract_version", "received_at", "parse.outcome"],
}


def object_schema(attributes, types):
    properties = {}
    required = []
    for name, rule in attributes.items():
        scalar = rule["type"]
        while parent := types.get(scalar, {}).get("type"):
            scalar = parent
        if target := rule.get("object_type"):
            shape = {"$ref": f"#/$defs/{target}"}
        else:
            shape = {"type": TYPES[scalar]}
        if "enum" in rule and scalar in ("integer_t", "long_t"):
            shape["enum"] = sorted(map(int, rule["enum"]))
        if rule.get("is_array", False):
            shape = {"type": "array", "items": shape}
        properties[name] = shape
        if rule.get("requirement") == "required" and not rule.get("profiles"):
            required.append(name)
    return {"type": "object", "properties": properties, "required": required, "additionalProperties": False}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("compiled_schema", type=Path)
    parser.add_argument("corpus", type=Path)
    args = parser.parse_args()
    upstream = json.loads(args.compiled_schema.read_text())
    contract = json.loads(Path(__file__).with_name("event-contract.json").read_text())
    if (upstream["version"], upstream["compile_version"]) != (contract["upstream"]["version"], 1):
        parser.error("The validator requires the pinned OCSF input")

    types = upstream["dictionary"]["types"]["attributes"]
    definitions = {name: object_schema(value["attributes"], types) for name, value in upstream["objects"].items()}
    definitions["metadata"]["properties"]["spx"] = EXTENSION
    definitions["metadata"]["required"].append("spx")
    classes = {value["uid"]: value for value in upstream["classes"].values() if "uid" in value}
    classes[0] = upstream["classes"]["base_event"]
    events = json.loads(args.corpus.read_text())
    for event in events:
        number = event["class_uid"]
        document = object_schema(classes[number]["attributes"], types)
        schema = document | {"$defs": definitions}
        problems = list(Draft202012Validator(schema).iter_errors(event))
        if problems:
            for issue in problems:
                print(f"OCSF class {number} {issue.json_path}: {issue.message}")
            parser.exit(1)
    print(f"Validated {len(events)} published events with the pinned OCSF schema")


if __name__ == "__main__":
    main()
