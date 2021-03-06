 

/// <reference path="./typings.d.ts" />

import React from "react";
import * as d3 from "d3";

import shapes from "./world.json";

interface WorldMapProps {
  projection: d3.geo.Projection;
}

export class WorldMap extends React.Component<WorldMapProps> {
  render() {
    const pathGen = d3.geo.path().projection(this.props.projection);
    return (
      <g>
        <g>
          {shapes.features.map((feature: any, i: number) =>
            <path
              key={i}
              className="geopath"
              id={`world-${feature.id}`}
              d={pathGen(feature)}
            />,
          )}
        </g>
      </g>
    );
  }
}
